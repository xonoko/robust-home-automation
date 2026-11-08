//go:build linux

package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// runDevice simulates a device: it keeps one connection, sends a numbered
// 8-byte frame every interval and checks every echo arrives, in order.
// It never reconnects: any connection error is a test failure.
func runDevice(args []string) error {
	fs := flag.NewFlagSet("device", flag.ExitOnError)
	addr := fs.String("addr", "172.30.0.100:7000", "server address")
	interval := fs.Duration("interval", 20*time.Millisecond, "time between frames")
	_ = fs.Parse(args)

	var conn net.Conn
	var err error
	for range 50 { // the server may not be up yet; only the first connect may retry
		if conn, err = net.DialTimeout("tcp4", *addr, time.Second); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	fmt.Println("connected", conn.LocalAddr(), "->", conn.RemoteAddr())

	var (
		mu       sync.Mutex
		sentAt   = map[uint64]time.Time{}
		sent     uint64
		echoed   uint64
		outOfOrd uint64
		maxRTT   time.Duration
		failure  error
		secMax   time.Duration
	)
	fail := func(e error) {
		mu.Lock()
		if failure == nil {
			failure = e
		}
		mu.Unlock()
	}

	go func() { // receiver
		frame := make([]byte, 8)
		for {
			if _, err := io.ReadFull(conn, frame); err != nil {
				fail(fmt.Errorf("read: %w", err))
				return
			}
			seq := binary.BigEndian.Uint64(frame)
			now := time.Now()
			mu.Lock()
			if seq != echoed {
				outOfOrd++
			}
			echoed = seq + 1
			rtt := now.Sub(sentAt[seq])
			delete(sentAt, seq)
			maxRTT = max(maxRTT, rtt)
			secMax = max(secMax, rtt)
			mu.Unlock()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	tick := time.NewTicker(*interval)
	report := time.NewTicker(time.Second)
	frame := make([]byte, 8)
	for {
		select {
		case <-tick.C:
			mu.Lock()
			seq := sent
			sentAt[seq] = time.Now()
			sent++
			mu.Unlock()
			binary.BigEndian.PutUint64(frame, seq)
			if _, err := conn.Write(frame); err != nil {
				fail(fmt.Errorf("write: %w", err))
			}
		case <-report.C:
			mu.Lock()
			fmt.Printf("sent=%d echoed=%d max_rtt_last_second=%s\n", sent, echoed, secMax)
			secMax = 0
			f := failure
			mu.Unlock()
			if f != nil {
				return f
			}
		case <-sig:
			time.Sleep(500 * time.Millisecond) // let in-flight echoes arrive
			mu.Lock()
			defer mu.Unlock()
			summary := map[string]any{
				"sent": sent, "echoed": echoed, "lost": sent - echoed,
				"out_of_order": outOfOrd, "max_rtt": maxRTT.String(),
				"error": fmt.Sprint(failure), "reconnects": 0,
			}
			b, _ := json.Marshal(summary)
			fmt.Println("SUMMARY", string(b))
			if failure != nil || outOfOrd > 0 || sent != echoed {
				os.Exit(1)
			}
			return nil
		}
	}
}
