package forward

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/ssh"
)

// RemoteConfig 描述一条 -R 远程转发（内网穿透：在服务器上监听，
// 连接经 SSH 隧道回到本地目标）。
type RemoteConfig struct {
	// 服务器侧监听地址。
	BindHost string
	BindPort int
	// 服务器侧 Unix socket（可选，与 BindPort 二选一）。
	BindSocket string

	// 回连的本地目标。
	TargetNetwork string // "tcp" | "unix"
	TargetAddr    string
}

// Remote 实现 -R：SSH 服务器侧监听，连接回到本机目标。
type Remote struct {
	cfg RemoteConfig
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

// NewRemote 创建远程转发器。
func NewRemote(cli *ssh.Client, cfg RemoteConfig, t Traffic) *Remote {
	if cfg.TargetNetwork == "" {
		cfg.TargetNetwork = "tcp"
	}
	return &Remote{cfg: cfg, cli: cli, t: t, conns: make(map[net.Conn]struct{}), done: make(chan struct{})}
}

func (f *Remote) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return errors.New("forward: already stopped")
	}
	if f.started {
		return errors.New("forward: already started")
	}
	listenNetwork, listenAddr := "tcp", fmtAddr(f.cfg.BindHost, f.cfg.BindPort)
	if f.cfg.BindSocket != "" {
		listenNetwork, listenAddr = "unix", f.cfg.BindSocket
	}
	ln, err := f.cli.Listen(listenNetwork, listenAddr)
	if err != nil {
		return fmt.Errorf("forward: remote listen %s %s: %w", listenNetwork, listenAddr, err)
	}
	f.listener, f.started = ln, true
	go f.acceptLoop(ln)
	go f.watchClient()
	return nil
}

func (f *Remote) watchClient() {
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

func (f *Remote) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			f.mu.Lock()
			if !f.stopped {
				f.err = fmt.Errorf("forward: remote accept: %w", err)
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
func (f *Remote) SetLogger(l func(format string, args ...any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logf = l
}

func (f *Remote) handle(inbound net.Conn) {
	defer func() {
		f.mu.Lock()
		delete(f.conns, inbound)
		f.mu.Unlock()
	}()

	f.mu.Lock()
	logger := f.logf
	f.mu.Unlock()

	remoteAddr := inbound.RemoteAddr().String()
	if logger != nil {
		logger("Inbound remote connection from %s -> forwarding to local %s", remoteAddr, f.cfg.TargetAddr)
	}

	var target net.Conn
	var err error
	if f.cfg.TargetNetwork == "unix" {
		target, err = net.DialTimeout("unix", f.cfg.TargetAddr, 15*time.Second)
	} else {
		target, err = net.DialTimeout("tcp", f.cfg.TargetAddr, 15*time.Second)
	}
	if err != nil {
		if logger != nil {
			logger("Failed to connect to local target %s: %v", f.cfg.TargetAddr, err)
		}
		_ = inbound.Close()
		return
	}
	f.t.ConnOpened()
	if logger != nil {
		logger("Remote connection active: %s <-> %s", remoteAddr, f.cfg.TargetAddr)
	}
	// Remote peer -> local target is received traffic; the reverse direction is transmitted.
	// Only wrap inbound to ensure each byte in either direction is counted exactly once.
	cin := &countingConn{ReadWriteCloser: inbound, t: f.t, readTx: false}
	pipeConns(cin, target, f.t)
	f.t.ConnClosed()
	if logger != nil {
		logger("Remote connection closed from %s", remoteAddr)
	}
}

func (f *Remote) Stop() error {
	f.finish()
	return nil
}

func (f *Remote) finish() { f.closeOnce.Do(func() { _ = f.StopNoFinish(); close(f.done) }) }

// StopNoFinish closes resources without recursively calling finish.
func (f *Remote) StopNoFinish() error {
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
	return nil
}

func (f *Remote) Done() <-chan struct{} { return f.done }
func (f *Remote) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}
func (f *Remote) LocalAddr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener != nil {
		return f.listener.Addr().String()
	}
	if f.cfg.BindSocket != "" {
		return f.cfg.BindSocket
	}
	return fmtAddr(f.cfg.BindHost, f.cfg.BindPort)
}
