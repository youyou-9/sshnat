package forward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/ssh"
)

// LocalConfig 描述一条 -L 本地转发。
type LocalConfig struct {
	// ListenNetwork: "tcp"（默认）或 "unix"。
	ListenNetwork string
	// ListenAddr: tcp 为 host:port；unix 为 socket 文件路径。
	ListenAddr string

	// TargetNetwork: "tcp"（默认）或 "unix"（服务器侧 socket，走
	// direct-streamlocal@openssh.com 通道）。
	TargetNetwork string
	// TargetAddr: 目标 host:port 或 unix 路径。
	TargetAddr string
}

// Local 实现 -L：在本地监听，把每个连接经 SSH 转发到目标地址。
type Local struct {
	cfg  LocalConfig
	cli  *ssh.Client
	t    Traffic
	logf func(format string, args ...any)

	mu        sync.Mutex
	listener  net.Listener
	conns     map[net.Conn]struct{}
	started   bool
	stopped   bool
	done      chan struct{}
	err       error
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
	socket    os.FileInfo // identity of the socket created by this listener
}

// NewLocal 创建本地转发器。调用 Start 前不占用任何资源。
func NewLocal(cli *ssh.Client, cfg LocalConfig, t Traffic) *Local {
	if cfg.ListenNetwork == "" {
		cfg.ListenNetwork = "tcp"
	}
	if cfg.TargetNetwork == "" {
		cfg.TargetNetwork = "tcp"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Local{
		cfg:    cfg,
		cli:    cli,
		t:      t,
		conns:  make(map[net.Conn]struct{}),
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (f *Local) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return errors.New("forward: already stopped")
	}
	if f.started {
		return errors.New("forward: already started")
	}
	if f.cli == nil {
		return errors.New("forward: SSH client is nil")
	}

	var ln net.Listener
	var err error
	switch f.cfg.ListenNetwork {
	case "unix":
		if err := os.MkdirAll(filepath.Dir(f.cfg.ListenAddr), 0o755); err != nil {
			return fmt.Errorf("forward: create socket dir: %w", err)
		}
		// Do not unlink an existing path, even a socket: it may be a live
		// listener belonging to another application. Stale sockets must be
		// removed explicitly by their owner before this listener can start.
		ln, err = net.Listen("unix", f.cfg.ListenAddr)
	default:
		ln, err = net.Listen("tcp", f.cfg.ListenAddr)
	}
	if err != nil {
		return fmt.Errorf("forward: listen %s %s: %w", f.cfg.ListenNetwork, f.cfg.ListenAddr, err)
	}

	f.listener = ln
	if unixListener, ok := ln.(*net.UnixListener); ok {
		// Go normally unlinks by pathname on Close. Record the identity and
		// clean up only that socket, so a replaced file/socket is preserved.
		unixListener.SetUnlinkOnClose(false)
		f.socket, _ = os.Lstat(f.cfg.ListenAddr)
	}
	f.started = true
	f.workers.Add(1)
	go f.acceptLoop(ln)
	go f.watchClient()
	return nil
}

func (f *Local) watchClient() {
	if f.cli == nil {
		return
	}
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

func (f *Local) acceptLoop(ln net.Listener) {
	defer f.workers.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			f.mu.Lock()
			stopped := f.stopped
			if !stopped {
				f.err = fmt.Errorf("forward: accept: %w", err)
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
func (f *Local) SetLogger(l func(format string, args ...any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logf = l
}

// handle 把一个入站连接经 SSH 拨到目标并双向搬运。
func (f *Local) handle(inbound net.Conn) {
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
		logger("Inbound connection from %s -> dialing target %s via SSH...", remoteAddr, f.cfg.TargetAddr)
	}

	target, err := f.dialTarget()
	if err != nil {
		if logger != nil {
			logger("Failed to connect to target %s via SSH: %v", f.cfg.TargetAddr, err)
		}
		// 目标不可达：直接关闭入站连接，不计入连接统计。
		_ = inbound.Close()
		return
	}

	f.t.ConnOpened()
	if logger != nil {
		logger("Connection active: %s <-> %s", remoteAddr, f.cfg.TargetAddr)
	}

	// Bytes read from the local side are sent to the SSH target (Tx); bytes
	// written back to the local side are received traffic (Rx).
	// Only wrap inbound to ensure each byte in either direction is counted exactly once.
	cin := &countingConn{ReadWriteCloser: inbound, t: f.t, readTx: true}
	pipeConns(f.ctx, cin, target)

	f.t.ConnClosed()
	if logger != nil {
		logger("Connection closed from %s", remoteAddr)
	}
}

// dialTarget 根据配置拨向 TCP 地址或服务器侧 Unix socket，带 10 秒超时。
func (f *Local) dialTarget() (net.Conn, error) {
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()

	return f.cli.DialContext(ctx, f.cfg.TargetNetwork, f.cfg.TargetAddr)
}

// Stop 关闭监听器与所有活跃连接，幂等。
func (f *Local) Stop() error {
	f.finish()
	<-f.done
	return nil
}

func (f *Local) finish() {
	f.closeOnce.Do(func() {
		_ = f.StopNoFinish()
		go func() {
			f.workers.Wait()
			close(f.done)
		}()
	})
}

// StopNoFinish 关闭底层资源但不重复调用 finish。
func (f *Local) StopNoFinish() error {
	f.mu.Lock()
	f.stopped = true
	f.cancel()
	ln := f.listener
	socket := f.socket
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
	if socket != nil {
		if current, err := os.Lstat(f.cfg.ListenAddr); err == nil && os.SameFile(socket, current) {
			_ = os.Remove(f.cfg.ListenAddr)
		}
	}
	return nil
}

func (f *Local) Done() <-chan struct{} { return f.done }

func (f *Local) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *Local) LocalAddr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener != nil {
		return f.listener.Addr().String()
	}
	return f.cfg.ListenAddr
}
