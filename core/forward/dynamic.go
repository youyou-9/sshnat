package forward

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/ssh"
)

// DynamicConfig 描述一条 -D 动态转发（本地 SOCKS5 代理）。
type DynamicConfig struct {
	ListenNetwork string // "tcp"（默认）
	ListenAddr    string // host:port
}

// Dynamic 实现 -D。本地 SOCKS5 代理通过 SSH 客户端连接目标。
type Dynamic struct {
	cfg DynamicConfig
	cli *ssh.Client
	t   Traffic
	logf func(format string, args ...any)

	mu        sync.Mutex
	listener  net.Listener
	conns     map[net.Conn]struct{}
	started   bool
	stopped   bool
	done      chan struct{}
	err       error
	closeOnce sync.Once
}

// NewDynamic 创建动态转发器。
func NewDynamic(cli *ssh.Client, cfg DynamicConfig, t Traffic) *Dynamic {
	if cfg.ListenNetwork == "" {
		cfg.ListenNetwork = "tcp"
	}
	return &Dynamic{cfg: cfg, cli: cli, t: t, conns: make(map[net.Conn]struct{}), done: make(chan struct{})}
}

func (f *Dynamic) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return errors.New("forward: already stopped")
	}
	if f.started {
		return errors.New("forward: already started")
	}
	ln, err := net.Listen(f.cfg.ListenNetwork, f.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("forward: listen SOCKS %s: %w", f.cfg.ListenAddr, err)
	}
	f.listener, f.started = ln, true
	go f.acceptLoop(ln)
	go f.watchClient()
	return nil
}

func (f *Dynamic) watchClient() {
	select {
	case <-f.cli.Done():
		f.mu.Lock()
		if f.err == nil && !f.stopped {
			f.err = errors.New("forward: ssh connection closed")
		}
		f.mu.Unlock()
		f.finish()
	case <-f.done:
	}
}

func (f *Dynamic) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			f.mu.Lock()
			if !f.stopped {
				f.err = fmt.Errorf("forward: SOCKS accept: %w", err)
			}
			f.mu.Unlock()
			f.finish()
			return
		}
		f.mu.Lock()
		if f.stopped {
			f.mu.Unlock()
			_ = conn.Close()
			return
		}
		f.conns[conn] = struct{}{}
		f.mu.Unlock()
		go f.handle(conn)
	}
}

// SetLogger 设置转发器日志输出函数。
func (f *Dynamic) SetLogger(l func(format string, args ...any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logf = l
}

func (f *Dynamic) handle(client net.Conn) {
	defer func() { f.mu.Lock(); delete(f.conns, client); f.mu.Unlock() }()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))

	f.mu.Lock()
	logger := f.logf
	f.mu.Unlock()

	if err := socks5Handshake(client); err != nil {
		if logger != nil {
			logger("SOCKS5 handshake failed: %v", err)
		}
		_ = client.Close()
		return
	}
	targetAddr, err := readSocks5Request(client)
	if err != nil {
		if logger != nil {
			logger("SOCKS5 invalid request: %v", err)
		}
		_ = client.Close()
		return
	}
	_ = client.SetDeadline(time.Time{})
	if logger != nil {
		logger("SOCKS5 proxy request for target %s", targetAddr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target, err := f.cli.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		if logger != nil {
			logger("SOCKS5 failed to dial target %s via SSH: %v", targetAddr, err)
		}
		_ = writeSocks5Reply(client, 0x05)
		_ = client.Close()
		return
	}
	if err := writeSocks5Reply(client, 0x00); err != nil {
		_ = target.Close()
		_ = client.Close()
		return
	}
	f.t.ConnOpened()
	if logger != nil {
		logger("SOCKS5 connection established to %s", targetAddr)
	}
	in := &countingConn{ReadWriteCloser: client, t: f.t, readTx: true}
	pipeConns(in, target, f.t)
	f.t.ConnClosed()
	if logger != nil {
		logger("SOCKS5 connection to %s closed", targetAddr)
	}
}

func (f *Dynamic) Stop() error {
	f.mu.Lock()
	f.stopped = true
	ln := f.listener
	conns := make([]net.Conn, 0, len(f.conns))
	for c := range f.conns {
		conns = append(conns, c)
	}
	f.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
	f.finish()
	return nil
}

func (f *Dynamic) finish() { f.closeOnce.Do(func() { _ = f.stopResources(); close(f.done) }) }
func (f *Dynamic) stopResources() error {
	f.mu.Lock()
	ln := f.listener
	conns := make([]net.Conn, 0, len(f.conns))
	for c := range f.conns {
		conns = append(conns, c)
	}
	f.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
	return nil
}
func (f *Dynamic) Done() <-chan struct{} { return f.done }
func (f *Dynamic) Err() error            { f.mu.Lock(); defer f.mu.Unlock(); return f.err }
func (f *Dynamic) LocalAddr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener != nil {
		return f.listener.Addr().String()
	}
	return f.cfg.ListenAddr
}

func socks5Handshake(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 {
		return errors.New("forward: invalid SOCKS5 greeting")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	for _, method := range methods {
		if method == 0 {
			_, err := conn.Write([]byte{5, 0})
			return err
		}
	}
	_, err := conn.Write([]byte{5, 0xff})
	if err != nil {
		return fmt.Errorf("forward: SOCKS5 rejection write failed: %w", err)
	}
	return errors.New("forward: SOCKS5 authentication not supported")
}

func readSocks5Request(conn net.Conn) (string, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(conn, h); err != nil {
		return "", err
	}
	if h[0] != 5 || h[1] != 1 || h[2] != 0 {
		_ = writeSocks5Reply(conn, 0x07)
		return "", errors.New("forward: unsupported SOCKS5 request")
	}
	var host string
	switch h[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case 3:
		b := []byte{0}
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		name := make([]byte, int(b[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			return "", err
		}
		host = string(name)
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	default:
		_ = writeSocks5Reply(conn, 0x08)
		return "", errors.New("forward: unsupported SOCKS5 address type")
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port)))), nil
}

func writeSocks5Reply(conn net.Conn, code byte) error {
	return writeSocks5Addr(conn, code, 1, []byte{0, 0, 0, 0}, 0)
}

func writeSocks5Addr(conn net.Conn, code, atyp byte, host []byte, port uint16) error {
	buf := []byte{5, code, 0, atyp}
	if atyp == 3 {
		buf = append(buf, byte(len(host)))
	}
	buf = append(buf, host...)
	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, port)
	buf = append(buf, p...)
	_, err := conn.Write(buf)
	return err
}

func fmtAddr(host string, port int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
