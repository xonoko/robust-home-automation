//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Values from include/uapi/linux/tcp.h that x/sys does not export.
const (
	tcpRecvQueue = 1
	tcpSendQueue = 2

	tcpoptMSS       = 2
	tcpoptWindow    = 3
	tcpoptSACKPerm  = 4
	tcpoptTimestamp = 8

	tcpiOptTimestamps = 1
	tcpiOptSACK       = 2
	tcpiOptWscale     = 4
)

// RepairWindow mirrors struct tcp_repair_window.
type RepairWindow struct {
	SndWl1    uint32
	SndWnd    uint32
	MaxWindow uint32
	RcvWnd    uint32
	RcvWup    uint32
}

// SocketState is everything needed to rebuild an established TCP connection
// on another machine that owns the same local address.
type SocketState struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`

	// Sequence number after the last byte written (send queue) or received (receive queue).
	SndSeq uint32 `json:"sndSeq"`
	RcvSeq uint32 `json:"rcvSeq"`
	// Bytes not yet acknowledged by the peer (sent or not), and bytes received but not read.
	SendQueue []byte `json:"sendQueue"`
	RecvQueue []byte `json:"recvQueue"`

	MSS        uint32 `json:"mss"`
	Timestamps bool   `json:"timestamps"`
	SACK       bool   `json:"sack"`
	Wscale     bool   `json:"wscale"`
	SndWscale  uint8  `json:"sndWscale"`
	RcvWscale  uint8  `json:"rcvWscale"`
	TSVal      uint32 `json:"tsval"`

	Window RepairWindow `json:"window"`
}

// dump puts the socket in repair mode and reads its state. The socket stays
// in repair mode: closing it afterwards sends nothing to the peer.
func dump(c *net.TCPConn) (SocketState, error) {
	st := SocketState{Local: c.LocalAddr().String(), Remote: c.RemoteAddr().String()}
	raw, err := c.SyscallConn()
	if err != nil {
		return st, err
	}
	var derr error
	if err := raw.Control(func(fd uintptr) { derr = dumpFD(int(fd), &st) }); err != nil {
		return st, err
	}
	return st, derr
}

func dumpFD(fd int, st *SocketState) error {
	if err := setInt(fd, unix.TCP_REPAIR, 1); err != nil {
		return fmt.Errorf("enter repair mode: %w", err)
	}

	var err error
	if st.RcvSeq, st.RecvQueue, err = dumpQueue(fd, tcpRecvQueue, unix.SIOCINQ); err != nil {
		return fmt.Errorf("receive queue: %w", err)
	}
	if st.SndSeq, st.SendQueue, err = dumpQueue(fd, tcpSendQueue, unix.SIOCOUTQ); err != nil {
		return fmt.Errorf("send queue: %w", err)
	}

	// struct tcp_info: byte 5 = tcpi_options, byte 6 = snd_wscale:4 | rcv_wscale:4, bytes 16..20 = tcpi_snd_mss.
	info := make([]byte, 232)
	n, err := getsockopt(fd, unix.IPPROTO_TCP, unix.TCP_INFO, info)
	if err != nil || n < 20 {
		return fmt.Errorf("tcp_info: %w (len %d)", err, n)
	}
	opts := info[5]
	st.Timestamps = opts&tcpiOptTimestamps != 0
	st.SACK = opts&tcpiOptSACK != 0
	st.Wscale = opts&tcpiOptWscale != 0
	st.SndWscale = info[6] & 0x0f
	st.RcvWscale = info[6] >> 4
	st.MSS = binary.NativeEndian.Uint32(info[16:20])

	if st.Timestamps {
		v, err := unix.GetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_TIMESTAMP)
		if err != nil {
			return fmt.Errorf("timestamp: %w", err)
		}
		st.TSVal = uint32(v)
	}

	win := make([]byte, unsafe.Sizeof(RepairWindow{}))
	if _, err := getsockopt(fd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW, win); err != nil {
		return fmt.Errorf("window: %w", err)
	}
	st.Window = RepairWindow{
		SndWl1:    binary.NativeEndian.Uint32(win[0:]),
		SndWnd:    binary.NativeEndian.Uint32(win[4:]),
		MaxWindow: binary.NativeEndian.Uint32(win[8:]),
		RcvWnd:    binary.NativeEndian.Uint32(win[12:]),
		RcvWup:    binary.NativeEndian.Uint32(win[16:]),
	}
	return nil
}

func dumpQueue(fd, queue int, sizeIoctl uint) (uint32, []byte, error) {
	if err := setInt(fd, unix.TCP_REPAIR_QUEUE, queue); err != nil {
		return 0, nil, err
	}
	seq, err := unix.GetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ)
	if err != nil {
		return 0, nil, err
	}
	size, err := unix.IoctlGetInt(fd, sizeIoctl)
	if err != nil {
		return 0, nil, err
	}
	if size == 0 {
		return uint32(seq), nil, nil
	}
	buf := make([]byte, size)
	n, _, err := unix.Recvfrom(fd, buf, unix.MSG_PEEK|unix.MSG_DONTWAIT)
	if err != nil {
		return 0, nil, fmt.Errorf("peek: %w", err)
	}
	if n != size {
		return 0, nil, fmt.Errorf("peeked %d of %d bytes", n, size)
	}
	return uint32(seq), buf, nil
}

// restore rebuilds an established connection from st. The local address must
// already be configured on this machine.
func restore(st SocketState) (*net.TCPConn, error) {
	local, err := netip.ParseAddrPort(st.Local)
	if err != nil {
		return nil, err
	}
	remote, err := netip.ParseAddrPort(st.Remote)
	if err != nil {
		return nil, err
	}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = unix.Close(fd)
		}
	}()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"reuseaddr", func() error { return unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1) }},
		{"repair on", func() error { return setInt(fd, unix.TCP_REPAIR, 1) }},
		// Sequence numbers are set to the start of each queue; writing the
		// queued bytes below advances them to the dumped values.
		{"recv seq", func() error { return setQueueSeq(fd, tcpRecvQueue, st.RcvSeq-uint32(len(st.RecvQueue))) }},
		{"send seq", func() error { return setQueueSeq(fd, tcpSendQueue, st.SndSeq-uint32(len(st.SendQueue))) }},
		{"bind", func() error { return unix.Bind(fd, sockaddr(local)) }},
		// In repair mode connect() sends no SYN and puts the socket in ESTABLISHED.
		{"connect", func() error { return unix.Connect(fd, sockaddr(remote)) }},
		{"options", func() error { return setRepairOptions(fd, st) }},
		{"timestamp", func() error {
			if !st.Timestamps {
				return nil
			}
			return setInt(fd, unix.TCP_TIMESTAMP, int(st.TSVal))
		}},
		{"recv queue", func() error { return writeQueue(fd, tcpRecvQueue, st.RecvQueue) }},
		{"send queue", func() error { return writeQueue(fd, tcpSendQueue, st.SendQueue) }},
		{"window", func() error { return setWindow(fd, st.Window) }},
		// Leaving repair mode sends a window probe, which prompts the peer to retransmit.
		{"repair off", func() error { return setInt(fd, unix.TCP_REPAIR, 0) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return nil, fmt.Errorf("restore %s: %w", s.name, err)
		}
	}

	f := os.NewFile(uintptr(fd), "restored-tcp")
	ok = true
	nc, err := net.FileConn(f) // duplicates the descriptor
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return nc.(*net.TCPConn), nil
}

func setQueueSeq(fd, queue int, seq uint32) error {
	if err := setInt(fd, unix.TCP_REPAIR_QUEUE, queue); err != nil {
		return err
	}
	return setInt(fd, unix.TCP_QUEUE_SEQ, int(int32(seq)))
}

func writeQueue(fd, queue int, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := setInt(fd, unix.TCP_REPAIR_QUEUE, queue); err != nil {
		return err
	}
	for len(data) > 0 {
		n, err := unix.Write(fd, data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func setRepairOptions(fd int, st SocketState) error {
	type opt struct{ code, val uint32 }
	opts := []opt{{tcpoptMSS, st.MSS}}
	if st.Wscale {
		opts = append(opts, opt{tcpoptWindow, uint32(st.SndWscale) | uint32(st.RcvWscale)<<16})
	}
	if st.SACK {
		opts = append(opts, opt{tcpoptSACKPerm, 0})
	}
	if st.Timestamps {
		opts = append(opts, opt{tcpoptTimestamp, 0})
	}
	buf := make([]byte, 8*len(opts))
	for i, o := range opts {
		binary.NativeEndian.PutUint32(buf[8*i:], o.code)
		binary.NativeEndian.PutUint32(buf[8*i+4:], o.val)
	}
	return setsockopt(fd, unix.IPPROTO_TCP, unix.TCP_REPAIR_OPTIONS, buf)
}

func setWindow(fd int, w RepairWindow) error {
	buf := make([]byte, 20)
	for i, v := range []uint32{w.SndWl1, w.SndWnd, w.MaxWindow, w.RcvWnd, w.RcvWup} {
		binary.NativeEndian.PutUint32(buf[4*i:], v)
	}
	return setsockopt(fd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW, buf)
}

func setInt(fd, opt, v int) error { return unix.SetsockoptInt(fd, unix.IPPROTO_TCP, opt, v) }

func sockaddr(a netip.AddrPort) unix.Sockaddr {
	return &unix.SockaddrInet4{Port: int(a.Port()), Addr: a.Addr().As4()}
}

func getsockopt(fd, level, opt int, buf []byte) (int, error) {
	l := uint32(len(buf))
	_, _, e := unix.Syscall6(unix.SYS_GETSOCKOPT, uintptr(fd), uintptr(level), uintptr(opt),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&l)), 0)
	if e != 0 {
		return 0, e
	}
	return int(l), nil
}

func setsockopt(fd, level, opt int, buf []byte) error {
	_, _, e := unix.Syscall6(unix.SYS_SETSOCKOPT, uintptr(fd), uintptr(level), uintptr(opt),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if e != 0 {
		return e
	}
	return nil
}
