//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// node echoes 8-byte frames on vip:port when it owns the VIP, and moves its
// live connections to another node on POST /migrate.
type node struct {
	vip, vmac, iface string
	port             int
	log              *slog.Logger

	mu       sync.Mutex
	active   bool
	listener net.Listener
	conns    map[*echoConn]struct{}
}

// echoConn is one device connection plus its application state: bytes read
// that do not form a complete frame yet.
type echoConn struct {
	c       *net.TCPConn
	partial []byte
	frozen  chan struct{}
	done    chan struct{}
}

// migrationState is what a node sends to the next one.
type migrationState struct {
	Conns []connState `json:"conns"`
}

type connState struct {
	Socket  SocketState `json:"socket"`
	Partial []byte      `json:"partial"`
}

func runNode(args []string) error {
	fs := flag.NewFlagSet("node", flag.ExitOnError)
	n := &node{conns: map[*echoConn]struct{}{}}
	fs.StringVar(&n.vip, "vip", "172.30.0.100", "virtual IP devices connect to")
	fs.StringVar(&n.vmac, "vmac", "02:00:00:00:01:64", "virtual MAC that moves with the VIP")
	fs.StringVar(&n.iface, "iface", "eth0", "LAN interface")
	fs.IntVar(&n.port, "port", 7000, "service port")
	control := fs.String("control", ":9000", "control API address")
	active := fs.Bool("active", false, "own the VIP at start")
	name := fs.String("name", "node", "node name for logs")
	_ = fs.Parse(args)
	n.log = slog.New(slog.NewTextHandler(os.Stdout, nil)).With("node", *name)

	n.teardownVIP() // clean leftovers from a previous run
	if *active {
		if err := n.setupVIP(); err != nil {
			return err
		}
		if err := n.listen(); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /migrate", n.handleMigrate)
	mux.HandleFunc("POST /restore", n.handleRestore)
	n.log.Info("control API listening", "addr", *control, "active", *active)
	return http.ListenAndServe(*control, mux) //nolint:gosec // spike
}

func (n *node) listen() error {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) { serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1) })
		return errors.Join(err, serr)
	}}
	ln, err := lc.Listen(context.Background(), "tcp4", fmt.Sprintf("%s:%d", n.vip, n.port))
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.listener, n.active = ln, true
	n.mu.Unlock()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n.log.Info("device connected", "remote", c.RemoteAddr())
			n.serve(&echoConn{c: c.(*net.TCPConn)})
		}
	}()
	return nil
}

func (n *node) serve(ec *echoConn) {
	ec.frozen = make(chan struct{})
	ec.done = make(chan struct{})
	n.mu.Lock()
	n.conns[ec] = struct{}{}
	n.mu.Unlock()
	go func() {
		defer close(ec.done)
		buf := make([]byte, 4096)
		for {
			nr, err := ec.c.Read(buf)
			if nr > 0 {
				ec.partial = append(ec.partial, buf[:nr]...)
				if full := len(ec.partial) / 8 * 8; full > 0 {
					if _, werr := ec.c.Write(ec.partial[:full]); werr != nil {
						n.log.Error("write", "error", werr)
						return
					}
					ec.partial = append([]byte(nil), ec.partial[full:]...)
				}
			}
			if err != nil {
				select {
				case <-ec.frozen: // freeze requested: leave the socket to the migration
				default:
					n.log.Info("connection ended", "error", err)
					_ = ec.c.Close()
					n.mu.Lock()
					delete(n.conns, ec)
					n.mu.Unlock()
				}
				return
			}
		}
	}()
}

// handleMigrate moves every connection and the VIP to the target node.
func (n *node) handleMigrate(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("to")
	start := time.Now()
	timings := map[string]string{}
	mark := func(step string) { timings[step] = time.Since(start).String() }

	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.active {
		http.Error(w, "not active", http.StatusConflict)
		return
	}

	// 1. Network lock: drop incoming packets for the VIP so the socket state
	//    cannot change and the kernel never answers with a reset. Devices retransmit.
	if err := n.dropVIP(true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = n.listener.Close()

	// 2. Freeze the connection goroutines and dump each socket.
	var state migrationState
	for ec := range n.conns {
		close(ec.frozen)
		_ = ec.c.SetReadDeadline(time.Unix(1, 0))
		<-ec.done
		st, err := dump(ec.c)
		if err != nil {
			http.Error(w, "dump: "+err.Error(), http.StatusInternalServerError)
			return
		}
		state.Conns = append(state.Conns, connState{Socket: st, Partial: ec.partial})
	}
	mark("dumped")

	// 3. Release the VIP and its virtual MAC before the target claims them.
	n.teardownVIP()
	mark("vip_released")

	// 4. Hand the state to the target, which restores and claims the VIP.
	body, _ := json.Marshal(state)
	resp, err := http.Post(target+"/restore", "application/json", bytes.NewReader(body)) //nolint:gosec,noctx // spike
	if err != nil {
		http.Error(w, "restore: "+err.Error(), http.StatusBadGateway)
		return
	}
	targetTimings, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "restore failed: "+string(targetTimings), http.StatusBadGateway)
		return
	}
	mark("restored_on_target")

	// 5. Close our copies silently (repair mode) and lift the lock.
	for ec := range n.conns {
		_ = ec.c.Close()
		delete(n.conns, ec)
	}
	_ = n.dropVIP(false)
	n.active = false
	mark("done")

	n.log.Info("migrated", "connections", len(state.Conns), "to", target, "timings", timings)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connections": len(state.Conns), "bytes": len(body),
		"source": timings, "target": json.RawMessage(targetTimings),
	})
}

// handleRestore rebuilds the connections sent by the previous owner and claims the VIP.
func (n *node) handleRestore(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	timings := map[string]string{}
	mark := func(step string) { timings[step] = time.Since(start).String() }

	var state migrationState
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Drop VIP traffic until every socket exists, so early packets get no reset.
	if err := n.dropVIP(true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = n.dropVIP(false) }()

	if err := n.setupVIP(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mark("vip_claimed")

	// Listener first: a socket bound in repair mode ignores port conflicts,
	// but a listener cannot bind next to an established socket.
	if err := n.listen(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, cs := range state.Conns {
		c, err := restore(cs.Socket)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n.serve(&echoConn{c: c, partial: cs.Partial})
	}
	mark("sockets_restored")

	_ = n.dropVIP(false)
	// Gratuitous ARP from the virtual MAC: the switch learns the new port.
	// Devices' ARP caches are still valid since the MAC did not change.
	_ = run("arping", "-U", "-c", "2", "-I", "vip0", n.vip)
	mark("announced")

	n.log.Info("restored", "connections", len(state.Conns), "timings", timings)
	_ = json.NewEncoder(w).Encode(timings)
}

// setupVIP creates a macvlan interface with the virtual MAC and the VIP, and
// routes traffic sourced from the VIP through it.
func (n *node) setupVIP() error {
	cmds := [][]string{
		{"ip", "link", "add", "vip0", "link", n.iface, "type", "macvlan", "mode", "bridge"},
		{"ip", "link", "set", "vip0", "address", n.vmac},
		{"ip", "addr", "add", n.vip + "/32", "dev", "vip0"},
		{"ip", "link", "set", "vip0", "up"},
		{"ip", "rule", "add", "from", n.vip, "lookup", "100", "priority", "100"},
		{"ip", "route", "add", lanCIDR(n.vip), "dev", "vip0", "src", n.vip, "table", "100"},
	}
	for _, c := range cmds {
		if err := run(c...); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) teardownVIP() {
	_ = run("ip", "rule", "del", "from", n.vip, "lookup", "100")
	_ = run("ip", "route", "flush", "table", "100")
	_ = run("ip", "link", "del", "vip0")
}

func (n *node) dropVIP(on bool) error {
	action := "-I"
	if !on {
		action = "-D"
	}
	return run("iptables", action, "INPUT", "-d", n.vip, "-p", "tcp", "--dport", fmt.Sprint(n.port), "-j", "DROP")
}

// lanCIDR assumes a /24 LAN, which is what the compose file uses.
func lanCIDR(ip string) string {
	return ip[:strings.LastIndex(ip, ".")] + ".0/24"
}

func run(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput() //nolint:gosec // spike
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
