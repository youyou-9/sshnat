// Package supervisor 管理隧道生命周期：启停、指数退避自动重连、
// 健康探测（keepalive 由 core/ssh 提供）与事件广播。
//
// 并发模型：
//   - Supervisor 持有锁保护 tunnels 表；
//   - 每条运行中的隧道一个 goroutine（run loop），通过 stop channel 收敛；
//   - 状态变更一律经 setStatus 串行化并广播事件。
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/forward"
	"github.com/sshnat/sshnat/core/ssh"
	"github.com/sshnat/sshnat/core/stats"
)

// 重连退避参数。
const (
	baseBackoff = 1 * time.Second
	maxBackoff  = 30 * time.Second
)

// ErrNotFound 隧道不存在。
var ErrNotFound = errors.New("supervisor: tunnel not found")

// ErrAlreadyRunning 隧道已在运行。
var ErrAlreadyRunning = errors.New("supervisor: tunnel already running")

// ErrNotRunning 隧道未在运行。
var ErrNotRunning = errors.New("supervisor: tunnel not running")

// managed 是一条受管隧道的全部运行时状态。
type managed struct {
	tun config.Tunnel
	st  Status

	stopCh chan struct{} // 关闭即请求停止
	doneCh chan struct{} // run loop 退出后关闭
	once   sync.Once

	mu       sync.Mutex
	errText  string
	attempts int
}

// Supervisor 管理所有隧道的生命周期。
type Supervisor struct {
	mu       sync.Mutex
	tunnels  map[string]*managed
	store    *config.Store
	statsReg *stats.Registry

	events *bus

	dialTimeout time.Duration
	logf        func(format string, args ...any)
}

// New 创建 Supervisor。
func New(store *config.Store, reg *stats.Registry) *Supervisor {
	return &Supervisor{
		tunnels:     make(map[string]*managed),
		store:       store,
		statsReg:    reg,
		events:      newBus(),
		dialTimeout: 15 * time.Second,
		logf:        log.Printf,
	}
}

// Start 按 ID 启动隧道。幂等：已运行返回 ErrAlreadyRunning。
func (s *Supervisor) Start(id string) error {
	settings, err := s.store.Load()
	if err != nil {
		return err
	}
	tun, ok := settings.FindTunnel(id)
	if !ok {
		return ErrNotFound
	}
	host, ok := settings.FindHost(tun.HostID)
	if !ok {
		return fmt.Errorf("%w: host %s", ErrNotFound, tun.HostID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, running := s.tunnels[id]; running {
		status, _ := existing.currentStatus()
		if status != StatusError && status != StatusStopped {
			return ErrAlreadyRunning
		}
		delete(s.tunnels, id)
	}

	m := &managed{
		tun:    *tun,
		st:     StatusStarting,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	s.tunnels[id] = m
	s.statsReg.Reset(id)

	// 广播初始状态迁移（stopped -> starting）。
	// 注意 setStatus 内部会拿 m.mu，不会与 s.mu 死锁。
	go func() {
		m.setStatus(s, StatusStarting, "")
		s.run(m, *host)
	}()
	return nil
}

// Stop 停止隧道并等待其完全退出。未运行返回 ErrNotRunning。
func (s *Supervisor) Stop(id string) error {
	s.mu.Lock()
	m, ok := s.tunnels[id]
	if ok {
		m.setStatus(s, StatusStopping, "")
	}
	s.mu.Unlock()

	if !ok {
		return ErrNotRunning
	}
	m.once.Do(func() { close(m.stopCh) })
	select {
	case <-m.doneCh:
	case <-time.After(10 * time.Second):
		// run loop 卡死兜底：不再等待，直接从表中移除由调用方决定。
	}
	m.setStatus(s, StatusStopped, "")
	s.mu.Lock()
	delete(s.tunnels, id)
	s.mu.Unlock()
	return nil
}

// Status 返回隧道当前状态；未运行返回 StatusStopped。
func (s *Supervisor) Status(id string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.tunnels[id]; ok {
		return m.status()
	}
	return StatusStopped
}

// IsRunning 判断隧道是否在运行。
func (s *Supervisor) IsRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.tunnels[id]
	if !ok {
		return false
	}
	st := m.status()
	return st != StatusStopped && st != StatusError
}

// StopAll 停止所有运行中的隧道（进程退出时用）。
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.tunnels))
	for id := range s.tunnels {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(tid string) {
			defer wg.Done()
			_ = s.Stop(tid)
		}(id)
	}
	wg.Wait()
}

// StopAllWithTimeout 并行停止所有隧道，若在指定超时内未完成则强制返回。
func (s *Supervisor) StopAllWithTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	done := make(chan struct{})
	go func() {
		s.StopAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// status 读取状态（需持有 s.mu 或仅被 run goroutine 调用时安全）。
func (m *managed) status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// setStatus 更新状态并广播事件。可在任意 goroutine 调用。
func (m *managed) setStatus(s *Supervisor, next Status, errMsg string) {
	m.mu.Lock()
	prev := m.st
	m.st = next
	m.errText = errMsg
	m.mu.Unlock()

	s.events.emit(Event{
		Type:     EventStatus,
		TunnelID: m.tun.ID,
		Status:   next,
		Previous: prev,
		Error:    errMsg,
	})
}

func (m *managed) currentStatus() (Status, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st, m.errText
}

// EmitLog 广播一条指定隧道的日志事件。
func (s *Supervisor) EmitLog(tunnelID, message string) {
	s.events.emit(Event{
		Type:     EventLog,
		TunnelID: tunnelID,
		Message:  message,
	})
}

// run 是隧道的生命周期主循环：连接 → 转发 → 失败退避重连，直到 stop。
func (s *Supervisor) run(m *managed, host config.Host) {
	defer close(m.doneCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-m.stopCh:
			cancel()
		case <-m.doneCh:
		}
	}()

	s.events.emit(Event{
		Type:     EventLog,
		TunnelID: m.tun.ID,
		Message:  fmt.Sprintf("Starting tunnel '%s' (connecting to %s@%s:%d)...", m.tun.Name, host.User, host.Host, host.Port),
	})

	backoff := baseBackoff
	for {
		// ---- 连接阶段 ----
		client, err := s.dialHost(ctx, host)
		if err != nil {
			select {
			case <-m.stopCh:
				m.setStatus(s, StatusStopped, "stopped while connecting")
				s.events.emit(Event{
					Type:     EventLog,
					TunnelID: m.tun.ID,
					Message:  "Tunnel stopped while connecting",
				})
				return
			default:
			}

			// 首次连接失败也进入重连态：用户能看到错误与退避倒计时。
			m.mu.Lock()
			prev := m.st
			m.attempts++
			attempt := m.attempts
			m.st = StatusReconnecting
			m.errText = err.Error()
			m.mu.Unlock()

			s.events.emit(Event{
				Type:     EventStatus,
				TunnelID: m.tun.ID,
				Status:   StatusReconnecting,
				Previous: prev,
				Error:    err.Error(),
				Attempt:  attempt,
				NextInMs: int64(backoff / time.Millisecond),
			})
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  fmt.Sprintf("SSH connection failed: %v (retrying in %s)", err, backoff),
			})
			s.logf("[supervisor] tunnel %s connect failed: %v (retry in %s)", m.tun.ID, err, backoff)

			if !sleepOrStop(m.stopCh, backoff) {
				m.setStatus(s, StatusStopped, "stopped while connecting")
				s.events.emit(Event{
					Type:     EventLog,
					TunnelID: m.tun.ID,
					Message:  "Tunnel stopped by user",
				})
				return
			}
			backoff = min64(backoff*2, maxBackoff)
			continue
		}

		select {
		case <-m.stopCh:
			client.Close()
			m.setStatus(s, StatusStopped, "")
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  "Tunnel stopped by user",
			})
			return
		default:
		}

		// ---- 转发阶段 ----
		fw, err := s.buildForwarder(client, &m.tun, s.statsReg.Get(m.tun.ID))
		if err != nil {
			client.Close()
			s.logf("[supervisor] tunnel %s forwarder error: %v", m.tun.ID, err)
			m.setStatus(s, StatusError, err.Error())
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  fmt.Sprintf("Forwarder startup failed: %v", err),
			})
			return
		}
		if err := fw.Start(); err != nil {
			client.Close()
			s.logf("[supervisor] tunnel %s start error: %v", m.tun.ID, err)
			m.setStatus(s, StatusError, err.Error())
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  fmt.Sprintf("Forwarder start failed: %v", err),
			})
			return
		}

		backoff = baseBackoff
		m.mu.Lock()
		m.attempts = 0
		m.mu.Unlock()
		m.setStatus(s, StatusConnected, "")
		s.events.emit(Event{
			Type:     EventLog,
			TunnelID: m.tun.ID,
			Message:  fmt.Sprintf("Tunnel connected successfully, listening on %s", fw.LocalAddr()),
		})
		s.logf("[supervisor] tunnel %s connected (%s)", m.tun.ID, fw.LocalAddr())

		// ---- 运行阶段 ----
		select {
		case <-fw.Done():
			// 转发器终止（监听失败或 keepalive 探测到连接死亡）。
			ferr := fw.Err()
			client.Close()
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  fmt.Sprintf("Connection lost: %v", ferr),
			})
			s.logf("[supervisor] tunnel %s lost: %v", m.tun.ID, ferr)

			select {
			case <-m.stopCh:
				m.setStatus(s, StatusStopped, "")
				s.events.emit(Event{
					Type:     EventLog,
					TunnelID: m.tun.ID,
					Message:  "Tunnel stopped by user",
				})
				return
			default:
			}
			m.setStatus(s, StatusReconnecting, fmt.Sprint(ferr))
			if !sleepOrStop(m.stopCh, backoff) {
				m.setStatus(s, StatusStopped, "stopped while reconnecting")
				s.events.emit(Event{
					Type:     EventLog,
					TunnelID: m.tun.ID,
					Message:  "Tunnel stopped by user",
				})
				return
			}
			backoff = min64(backoff*2, maxBackoff)

		case <-m.stopCh:
			_ = fw.Stop()
			client.Close()
			m.setStatus(s, StatusStopped, "")
			s.events.emit(Event{
				Type:     EventLog,
				TunnelID: m.tun.ID,
				Message:  "Tunnel stopped by user",
			})
			return
		}
	}
}

// dialHost 建立 SSH 连接（含跳板链解析与认证参数转换）。
func (s *Supervisor) dialHost(parentCtx context.Context, host config.Host) (*ssh.Client, error) {
	opts, err := buildDialOptions(s.store, host)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parentCtx, s.dialTimeout)
	defer cancel()
	return ssh.Dial(ctx, opts)
}

// buildForwarder 根据隧道类型构造转发器。
func (s *Supervisor) buildForwarder(cli *ssh.Client, tun *config.Tunnel, t *stats.Counter) (forward.Forwarder, error) {
	logCallback := func(format string, args ...any) {
		s.EmitLog(tun.ID, fmt.Sprintf(format, args...))
	}
	switch tun.Type {
	case config.TypeLocal:
		listenNet, listenAddr := localListen(tun)
		targetNet, targetAddr := "tcp", ""
		if tun.TargetSocket != "" {
			targetNet, targetAddr = "unix", tun.TargetSocket
		} else {
			th := tun.TargetHost
			if th == "" {
				th = "127.0.0.1"
			}
			targetAddr = fmt.Sprintf("%s:%d", th, tun.TargetPort)
		}
		cfg := forward.LocalConfig{
			ListenNetwork: listenNet,
			ListenAddr:    listenAddr,
			TargetNetwork: targetNet,
			TargetAddr:    targetAddr,
		}
		f := forward.NewLocal(cli, cfg, t)
		f.SetLogger(logCallback)
		return f, nil

	case config.TypeRemote:
		bindHost := tun.RemoteBindHost
		if bindHost == "" {
			bindHost = "127.0.0.1"
		}
		targetHost := tun.TargetHost
		if targetHost == "" {
			targetHost = "127.0.0.1"
		}
		f := forward.NewRemote(cli, forward.RemoteConfig{
			BindHost:      bindHost,
			BindPort:      tun.RemotePort,
			TargetNetwork: "tcp",
			TargetAddr:    fmt.Sprintf("%s:%d", targetHost, tun.TargetPort),
		}, t)
		f.SetLogger(logCallback)
		return f, nil
	case config.TypeDynamic:
		bindHost := tun.LocalBindHost
		if bindHost == "" {
			bindHost = "127.0.0.1"
		}
		f := forward.NewDynamic(cli, forward.DynamicConfig{
			ListenNetwork: "tcp",
			ListenAddr:    fmt.Sprintf("%s:%d", bindHost, tun.SocksPort),
		}, t)
		f.SetLogger(logCallback)
		return f, nil
	default:
		return nil, fmt.Errorf("supervisor: unknown tunnel type %q", tun.Type)
	}
}

func localListen(tun *config.Tunnel) (string, string) {
	if tun.LocalSocket != "" {
		return "unix", tun.LocalSocket
	}
	bh := tun.LocalBindHost
	if bh == "" {
		bh = "127.0.0.1"
	}
	return "tcp", fmt.Sprintf("%s:%d", bh, tun.LocalPort)
}

// sleepOrStop 睡眠 d；期间收到停止信号返回 true，自然醒返回 false。
func sleepOrStop(stopCh <-chan struct{}, d time.Duration) bool {
	select {
	case <-stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

func min64(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
