package forward

import (
	"context"
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
	cfg  RemoteConfig
	cli  *ssh.Client
	t    Traffic
	logf func(format string, args ...any)

	mu        sync.Mutex
	listener  net.Listener
	conns     map[net.Conn]struct{}
	starting  bool
	started   bool
	stopped   bool
	done      chan struct{}
	err       error
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
}

// NewRemote 创建远程转发器。
func NewRemote(cli *ssh.Client, cfg RemoteConfig, t Traffic) *Remote {
	if cfg.TargetNetwork == "" {
		cfg.TargetNetwork = "tcp"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Remote{cfg: cfg, cli: cli, t: t, conns: make(map[net.Conn]struct{}), done: make(chan struct{}), ctx: ctx, cancel: cancel}
}

func (f *Remote) Start() error {
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return errors.New("forward: already stopped")
	}
	if f.starting || f.started {
		f.mu.Unlock()
		return errors.New("forward: already started")
	}
	if f.cli == nil {
		f.mu.Unlock()
		return errors.New("forward: SSH client is nil")
	}
	f.starting = true
	f.workers.Add(1)
	f.mu.Unlock()
	defer f.workers.Done()

	listenNetwork, listenAddr := "tcp", fmtAddr(f.cfg.BindHost, f.cfg.BindPort)
	type listenResult struct {
		listener net.Listener
		err      error
	}
	result := make(chan listenResult, 1)
	go func() {
		var ln net.Listener
		var err error
		if f.cfg.BindSocket != "" {
			ln, err = f.cli.ListenUnix(f.cfg.BindSocket)
		} else {
			ln, err = f.cli.Listen(listenNetwork, listenAddr)
		}
		result <- listenResult{listener: ln, err: err}
	}()
	var response listenResult
	select {
	case response = <-result:
	case <-f.ctx.Done():
		// Interrupt the global request so Stop can run while Start is pending.
		_ = f.cli.Close()
		response = <-result
	}
	f.mu.Lock()
	f.starting = false
	if f.stopped {
		f.mu.Unlock()
		if response.listener != nil {
			_ = f.cli.Close()
			_ = response.listener.Close()
		}
		return errors.New("forward: stopped while starting")
	}
	if response.err != nil {
		f.mu.Unlock()
		return fmt.Errorf("forward: remote listen %s %s: %w", listenNetwork, listenAddr, response.err)
	}
	f.listener, f.started = response.listener, true
	f.workers.Add(1)
	f.mu.Unlock()
	go f.acceptLoop(response.listener)
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
	defer f.workers.Done()
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
		f.workers.Add(1)
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
	defer f.workers.Done()
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

	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	target, err := dialer.DialContext(ctx, f.cfg.TargetNetwork, f.cfg.TargetAddr)
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
	pipeConns(f.ctx, cin, target)
	f.t.ConnClosed()
	if logger != nil {
		logger("Remote connection closed from %s", remoteAddr)
	}
}

func (f *Remote) Stop() error {
	f.finish()
	<-f.done
	return nil
}

func (f *Remote) finish() {
	f.closeOnce.Do(func() {
		_ = f.StopNoFinish()
		go func() {
			f.workers.Wait()
			close(f.done)
		}()
	})
}

// StopNoFinish closes resources without recursively calling finish.
func (f *Remote) StopNoFinish() error {
	f.mu.Lock()
	f.stopped = true
	f.cancel()
	ln := f.listener
	conns := make([]net.Conn, 0, len(f.conns))
	for c := range f.conns {
		conns = append(conns, c)
	}
	f.mu.Unlock()
	if ln != nil {
		closed := make(chan struct{})
		go func() { _ = ln.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(time.Second):
			// OpenSSH remote-listener Close waits for a global cancellation
			// reply. A dead/unresponsive server must not strand Stop forever;
			// closing the owning SSH transport releases that request.
			if f.cli != nil {
				_ = f.cli.Close()
			}
			<-closed
		}
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
