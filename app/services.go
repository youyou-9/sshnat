// Package app 是 SSHNat 的 Wails v3 绑定层。
//
// 架构铁律：本包只做参数转换与事件转发，所有业务逻辑都在 core/。
// 未来更换 UI 壳（Wails v2 / Tauri sidecar）时 core 不需要任何改动。
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
	"github.com/sshnat/sshnat/internal/version"
)

// Wails 事件名。
const (
	EventTunnelStatus  = "sshnat:tunnel-status"
	EventTunnelStats   = "sshnat:tunnel-stats"
	EventLog           = "sshnat:log"
	EventConfigChanged = "sshnat:config-changed"
)

// Emitter 由 main 装配：把 core 事件转发到前端。
type Emitter func(name string, data ...any)

// TunnelView 是隧道配置 + 运行时状态的组合视图。
type TunnelView struct {
	config.Tunnel
	Status  string         `json:"status"`
	Running bool           `json:"running"`
	Error   string         `json:"error"`
	Stats   stats.Snapshot `json:"stats"`
}

// CreateTunnelRequest 是 New Tunnel 弹窗的提交载荷。
type CreateTunnelRequest struct {
	Name   string `json:"name"`
	HostID string `json:"hostId"`
	Type   string `json:"type"` // L | R | D

	LocalBindHost string `json:"localBindHost,omitempty"`
	LocalPort     int    `json:"localPort,omitempty"`
	LocalSocket   string `json:"localSocket,omitempty"`

	TargetHost   string `json:"targetHost,omitempty"`
	TargetPort   int    `json:"targetPort,omitempty"`
	TargetSocket string `json:"targetSocket,omitempty"`

	RemoteBindHost string `json:"remoteBindHost,omitempty"`
	RemotePort     int    `json:"remotePort,omitempty"`
	RemoteSocket   string `json:"remoteSocket,omitempty"`

	SocksPort int  `json:"socksPort,omitempty"`
	AutoStart bool `json:"autoStart"`
}

// UpdateTunnelRequest 是编辑隧道的提交载荷。
type UpdateTunnelRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	HostID string `json:"hostId"`
	Type   string `json:"type"` // L | R | D

	LocalBindHost string `json:"localBindHost,omitempty"`
	LocalPort     int    `json:"localPort,omitempty"`
	LocalSocket   string `json:"localSocket,omitempty"`

	TargetHost   string `json:"targetHost,omitempty"`
	TargetPort   int    `json:"targetPort,omitempty"`
	TargetSocket string `json:"targetSocket,omitempty"`

	RemoteBindHost string `json:"remoteBindHost,omitempty"`
	RemotePort     int    `json:"remotePort,omitempty"`
	RemoteSocket   string `json:"remoteSocket,omitempty"`

	SocksPort int  `json:"socksPort,omitempty"`
	AutoStart bool `json:"autoStart"`
}

// Services 聚合全部绑定服务。
type Services struct {
	Store *config.Store
	Sup   *supervisor.Supervisor
	Stats *stats.Registry

	// Emit 在 main 中装配。
	emit         Emitter
	emu          sync.RWMutex
	chooseExport func() (string, error)

	tmu     sync.Mutex // 保护配置文件读写串行化
	closing bool       // also protected by tmu; shutdown rejects queued starts
}

// TunnelService 暴露隧道 CRUD 与启停。
type TunnelService struct{ s *Services }

// HostService 暴露主机 CRUD 与连通性测试。
type HostService struct{ s *Services }

// HostTestResult reports the outcome of a one-shot SSH connectivity check.
// A failed SSH handshake is represented in the result instead of as an RPC
// error so the UI can show a useful per-host status while keeping malformed
// requests (for example an unknown host ID) as regular API errors.
type HostTestResult struct {
	HostID     string `json:"hostId"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

const hostTestTimeout = 15 * time.Second

// SettingsService 暴露应用级设置。
type SettingsService struct{ s *Services }

// NewServices 创建并装配服务，同时启动 core 事件 → 前端事件的转发。
func NewServices(store *config.Store, sup *supervisor.Supervisor, reg *stats.Registry) *Services {
	s := &Services{Store: store, Sup: sup, Stats: reg}

	// 转发 supervisor 事件到前端。
	go func() {
		ch, cancel := sup.Subscribe()
		defer cancel()
		for ev := range ch {
			s.emu.RLock()
			emitter := s.emit
			s.emu.RUnlock()
			if emitter == nil {
				continue
			}
			switch ev.Type {
			case supervisor.EventStatus:
				emitter(EventTunnelStatus, map[string]any{
					"tunnelId": ev.TunnelID,
					"status":   string(ev.Status),
					"previous": string(ev.Previous),
					"error":    ev.Error,
					"attempt":  ev.Attempt,
					"nextInMs": ev.NextInMs,
				})
			case supervisor.EventStats:
				emitter(EventTunnelStats, map[string]any{
					"tunnelId":   ev.TunnelID,
					"tx":         ev.Tx,
					"rx":         ev.Rx,
					"txTotal":    ev.TxTotal,
					"rxTotal":    ev.RxTotal,
					"conns":      ev.Conns,
					"totalConns": ev.TotalConns,
				})
			case supervisor.EventLog:
				emitter(EventLog, map[string]any{
					"tunnelId": ev.TunnelID,
					"message":  ev.Message,
					"time":     ev.Time.Format("2006-01-02 15:04:05"),
				})
			}
		}
	}()

	return s
}

func (s *Services) tunnel() *TunnelService     { return &TunnelService{s} }
func (s *Services) host() *HostService         { return &HostService{s} }
func (s *Services) settings() *SettingsService { return &SettingsService{s} }

// ---------- TunnelService ----------

// Create 创建隧道并持久化。
func (t *TunnelService) Create(req CreateTunnelRequest) (*TunnelView, error) {
	if err := validateCreate(&req); err != nil {
		return nil, err
	}

	t.s.tmu.Lock()
	settings, err := t.s.Store.Load()
	if err != nil {
		t.s.tmu.Unlock()
		return nil, err
	}
	if _, ok := settings.FindHost(req.HostID); !ok {
		t.s.tmu.Unlock()
		return nil, fmt.Errorf("host %s not found", req.HostID)
	}
	tun := config.Tunnel{
		ID:             config.NewID("tnl"),
		Name:           req.Name,
		HostID:         req.HostID,
		Type:           req.Type,
		LocalBindHost:  req.LocalBindHost,
		LocalPort:      req.LocalPort,
		LocalSocket:    req.LocalSocket,
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		TargetSocket:   req.TargetSocket,
		RemoteBindHost: req.RemoteBindHost,
		RemotePort:     req.RemotePort,
		RemoteSocket:   req.RemoteSocket,
		SocksPort:      req.SocksPort,
		AutoStart:      req.AutoStart,
	}
	if tun.TargetHost == "" && tun.TargetSocket == "" && (tun.Type == config.TypeLocal || tun.Type == config.TypeRemote) {
		tun.TargetHost = "127.0.0.1"
	}
	settings.Tunnels = append(settings.Tunnels, tun)
	err = t.s.Store.Save(settings)
	t.s.tmu.Unlock()
	if err != nil {
		return nil, err
	}

	view := t.view(&tun)
	log.Printf("[app] tunnel created: %s (%s)", tun.ID, tun.Name)
	t.s.emitConfigChanged()
	return view, nil
}

// Update 更新已有隧道配置（运行中会先停止并在保存后重新启动）。
func (t *TunnelService) Update(req UpdateTunnelRequest) (*TunnelView, error) {
	createReq := CreateTunnelRequest{
		Name:           req.Name,
		HostID:         req.HostID,
		Type:           req.Type,
		LocalBindHost:  req.LocalBindHost,
		LocalPort:      req.LocalPort,
		LocalSocket:    req.LocalSocket,
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		TargetSocket:   req.TargetSocket,
		RemoteBindHost: req.RemoteBindHost,
		RemotePort:     req.RemotePort,
		RemoteSocket:   req.RemoteSocket,
		SocksPort:      req.SocksPort,
		AutoStart:      req.AutoStart,
	}
	if err := validateCreate(&createReq); err != nil {
		return nil, err
	}

	// Validate the target tunnel and host before stopping a running tunnel.
	// A typo in an update request must not stop a healthy tunnel first.
	t.s.tmu.Lock()
	changed := false
	defer func() {
		t.s.tmu.Unlock()
		if changed {
			t.s.emitConfigChanged()
		}
	}()
	currentSettings, err := t.s.Store.Load()
	if err != nil {
		return nil, err
	}
	if _, ok := currentSettings.FindTunnel(req.ID); !ok {
		return nil, fmt.Errorf("tunnel %s not found", req.ID)
	}
	if _, ok := currentSettings.FindHost(req.HostID); !ok {
		return nil, fmt.Errorf("host %s not found", req.HostID)
	}

	wasRunning := t.s.Sup.HasActiveRun(req.ID)
	if wasRunning {
		if err := t.s.Sup.Stop(req.ID); err != nil && !errors.Is(err, supervisor.ErrNotRunning) {
			return nil, fmt.Errorf("stop running tunnel for update: %w", err)
		}
	}

	settings, err := t.s.Store.Load()
	if err != nil {
		return nil, err
	}
	if _, ok := settings.FindHost(req.HostID); !ok {
		return nil, fmt.Errorf("host %s not found", req.HostID)
	}

	found := false
	var updated *config.Tunnel
	for i := range settings.Tunnels {
		if settings.Tunnels[i].ID == req.ID {
			settings.Tunnels[i] = config.Tunnel{
				ID:             req.ID,
				Name:           req.Name,
				HostID:         req.HostID,
				Type:           req.Type,
				LocalBindHost:  req.LocalBindHost,
				LocalPort:      req.LocalPort,
				LocalSocket:    req.LocalSocket,
				TargetHost:     req.TargetHost,
				TargetPort:     req.TargetPort,
				TargetSocket:   req.TargetSocket,
				RemoteBindHost: req.RemoteBindHost,
				RemotePort:     req.RemotePort,
				RemoteSocket:   req.RemoteSocket,
				SocksPort:      req.SocksPort,
				AutoStart:      req.AutoStart,
			}
			if settings.Tunnels[i].TargetHost == "" && settings.Tunnels[i].TargetSocket == "" && (settings.Tunnels[i].Type == config.TypeLocal || settings.Tunnels[i].Type == config.TypeRemote) {
				settings.Tunnels[i].TargetHost = "127.0.0.1"
			}
			updated = &settings.Tunnels[i]
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("tunnel %s not found", req.ID)
	}

	err = t.s.Store.Save(settings)
	if err != nil {
		return nil, err
	}
	changed = true

	if wasRunning {
		if err := t.s.Sup.Start(req.ID); err != nil {
			return nil, fmt.Errorf("restart updated tunnel: %w", err)
		}
	}

	log.Printf("[app] tunnel updated: %s (%s)", req.ID, req.Name)
	return t.view(updated), nil
}

// CreateFromSSHCommand 解析 `ssh -L ... user@host` 并创建主机 + 隧道。
// 返回新建的隧道视图列表。
func (t *TunnelService) CreateFromSSHCommand(cmd string) ([]TunnelView, error) {
	return t.CreateFromSSHCommandForShell(cmd, "")
}

// CreateFromSSHCommandForShell imports a command quoted for posix or
// powershell. An empty shell keeps the operating system's default.
func (t *TunnelService) CreateFromSSHCommandForShell(cmd, shell string) ([]TunnelView, error) {
	args, err := splitCommandForShell(cmd, shell)
	if err != nil {
		return nil, err
	}
	spec, err := config.ParseSSHCommand(args)
	if err != nil {
		return nil, err
	}
	host, jumpHosts, tunnels, err := config.SpecToSettingsWithJumps(spec)
	if err != nil {
		return nil, err
	}

	t.s.tmu.Lock()
	settings, err := t.s.Store.Load()
	if err != nil {
		t.s.tmu.Unlock()
		return nil, err
	}
	// Reuse existing jump hosts by address, while preserving the imported
	// ProxyJump order on the destination host.
	jumpIDs := make([]string, 0, len(jumpHosts))
	for i := range jumpHosts {
		jump := jumpHosts[i]
		if jump.Host == host.Host && jump.Port == host.Port && jump.User == host.User {
			t.s.tmu.Unlock()
			return nil, fmt.Errorf("ProxyJump chain cannot reference destination host")
		}
		if existing, ok := findHostByAddr(settings, jump.Host, jump.Port, jump.User); ok {
			jumpIDs = append(jumpIDs, existing.ID)
			continue
		}
		settings.Hosts = append(settings.Hosts, jump)
		jumpIDs = append(jumpIDs, jump.ID)
	}
	host.JumpHostIDs = jumpIDs
	// 同名主机复用，避免重复条目。
	if existing, ok := findHostByAddr(settings, host.Host, host.Port, host.User); ok {
		*host = existing
		spec.ApplyHostOptions(host)
		if len(jumpIDs) > 0 {
			host.JumpHostIDs = jumpIDs
		}
		for i := range settings.Hosts {
			if settings.Hosts[i].ID == host.ID {
				settings.Hosts[i] = *host
				break
			}
		}
	} else {
		settings.Hosts = append(settings.Hosts, *host)
	}
	for i := range tunnels {
		tunnels[i].HostID = host.ID
		settings.Tunnels = append(settings.Tunnels, tunnels[i])
	}
	err = t.s.Store.Save(settings)
	t.s.tmu.Unlock()
	if err != nil {
		return nil, err
	}

	views := make([]TunnelView, 0, len(tunnels))
	for i := range tunnels {
		views = append(views, *t.view(&tunnels[i]))
	}
	t.s.emitConfigChanged()
	return views, nil
}

// List 返回全部隧道及其实时状态。
func (t *TunnelService) List() ([]TunnelView, error) {
	settings, err := t.s.Store.Load()
	if err != nil {
		return nil, err
	}
	views := make([]TunnelView, 0, len(settings.Tunnels))
	for i := range settings.Tunnels {
		views = append(views, *t.view(&settings.Tunnels[i]))
	}
	return views, nil
}

// Get 按 ID 返回单个隧道视图。
func (t *TunnelService) Get(id string) (*TunnelView, error) {
	settings, err := t.s.Store.Load()
	if err != nil {
		return nil, err
	}
	tun, ok := settings.FindTunnel(id)
	if !ok {
		return nil, fmt.Errorf("tunnel %s not found", id)
	}
	return t.view(tun), nil
}

// Start 启动隧道。
func (t *TunnelService) Start(id string) error {
	t.s.tmu.Lock()
	defer t.s.tmu.Unlock()
	if t.s.closing {
		return errors.New("application is shutting down")
	}
	return t.s.Sup.Start(id)
}

// Stop 停止隧道。
func (t *TunnelService) Stop(id string) error { return t.s.Sup.Stop(id) }

// Delete 删除隧道（运行中会先停止）。
func (t *TunnelService) Delete(id string) error {
	t.s.tmu.Lock()
	changed := false
	defer func() {
		t.s.tmu.Unlock()
		if changed {
			t.s.emitConfigChanged()
		}
	}()
	if t.s.Sup.HasActiveRun(id) {
		if err := t.s.Sup.Stop(id); err != nil && !errors.Is(err, supervisor.ErrNotRunning) {
			return err
		}
	}
	settings, err := t.s.Store.Load()
	if err != nil {
		return err
	}
	kept := settings.Tunnels[:0]
	deleted := false
	for _, tun := range settings.Tunnels {
		if tun.ID == id {
			deleted = true
			continue
		}
		kept = append(kept, tun)
	}
	if !deleted {
		return fmt.Errorf("tunnel %s not found", id)
	}
	settings.Tunnels = kept
	if err := t.s.Store.Save(settings); err != nil {
		return err
	}
	t.s.Stats.Delete(id)
	changed = true
	return nil
}

// view 组装运行时视图。
func (t *TunnelService) view(tun *config.Tunnel) *TunnelView {
	st := t.s.Sup.Status(tun.ID)
	v := &TunnelView{
		Tunnel:  *tun,
		Status:  string(st),
		Running: t.s.Sup.IsRunning(tun.ID) && st != supervisor.StatusError && st != supervisor.StatusStopped,
		Error:   t.s.Sup.Error(tun.ID),
	}
	if st == supervisor.StatusError || st == supervisor.StatusReconnecting {
		// 错误详情由事件携带，这里仅标记状态。
	}
	if v.Running {
		v.Stats = t.s.Stats.Get(tun.ID).Snapshot()
	}
	return v
}

func validateCreate(req *CreateTunnelRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("tunnel name is required")
	}
	if strings.TrimSpace(req.HostID) == "" {
		return errors.New("host is required")
	}
	switch req.Type {
	case config.TypeLocal:
		if req.LocalSocket != "" && req.LocalPort != 0 {
			return errors.New("local port and local socket are mutually exclusive")
		}
		if req.TargetSocket != "" && (req.TargetHost != "" || req.TargetPort != 0) {
			return errors.New("target port and target socket are mutually exclusive")
		}
		if req.LocalPort == 0 && req.LocalSocket == "" {
			return errors.New("local port or socket is required")
		}
		if req.LocalPort != 0 && !validPort(req.LocalPort) {
			return errors.New("local port must be between 1 and 65535")
		}
		if req.TargetPort == 0 && req.TargetSocket == "" {
			return errors.New("target port is required")
		}
		if req.TargetPort != 0 && !validPort(req.TargetPort) {
			return errors.New("target port must be between 1 and 65535")
		}
	case config.TypeRemote:
		if req.RemoteSocket != "" && req.RemotePort != 0 {
			return errors.New("remote port and remote socket are mutually exclusive")
		}
		if req.TargetSocket != "" && (req.TargetHost != "" || req.TargetPort != 0) {
			return errors.New("target port and target socket are mutually exclusive")
		}
		if req.RemotePort == 0 && req.RemoteSocket == "" {
			return errors.New("remote port or socket is required")
		}
		if req.RemoteSocket == "" && !validPort(req.RemotePort) {
			return errors.New("remote port must be between 1 and 65535")
		}
		if req.TargetPort == 0 && req.TargetSocket == "" {
			return errors.New("target port or socket is required")
		}
		if req.TargetSocket == "" && !validPort(req.TargetPort) {
			return errors.New("target port must be between 1 and 65535")
		}
	case config.TypeDynamic:
		if req.SocksPort == 0 {
			return errors.New("SOCKS port is required")
		}
		if !validPort(req.SocksPort) {
			return errors.New("SOCKS port must be between 1 and 65535")
		}
	default:
		return fmt.Errorf("unknown tunnel type %q", req.Type)
	}
	return nil
}

func findHostByAddr(settings *config.Settings, host string, port int, user string) (config.Host, bool) {
	for _, h := range settings.Hosts {
		if h.Host == host && (h.Port == port || port == 22 && h.Port == 0) && h.User == user {
			return h, true
		}
	}
	return config.Host{}, false
}

// ---------- HostService ----------

// ListHosts 返回全部主机。
func (h *HostService) List() ([]config.Host, error) {
	settings, err := h.s.Store.Load()
	if err != nil {
		return nil, err
	}
	return settings.Hosts, nil
}

// TestConnection performs a bounded, one-shot SSH handshake for a saved host.
// It follows the same ProxyJump/authentication/host-key path as tunnels, but
// does not register a runtime tunnel and closes the client immediately after
// a successful handshake.
func (h *HostService) TestConnection(id string) (*HostTestResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("host id is required")
	}

	h.s.tmu.Lock()
	settings, err := h.s.Store.Load()
	if err != nil {
		h.s.tmu.Unlock()
		return nil, err
	}
	host, ok := settings.FindHost(id)
	if !ok {
		h.s.tmu.Unlock()
		return nil, fmt.Errorf("host %s not found", id)
	}
	// Copy the value before releasing the configuration lock. Dialing can take
	// several seconds and must not block saves or other host operations.
	hostCopy := *host
	h.s.tmu.Unlock()

	started := time.Now()
	result := &HostTestResult{HostID: id}
	timeout := hostTestTimeout
	if hostCopy.ConnectTimeoutSeconds > 0 {
		timeout = time.Duration(hostCopy.ConnectTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dialErr := h.s.Sup.TestHost(ctx, hostCopy)
	result.DurationMs = time.Since(started).Milliseconds()
	if dialErr != nil {
		result.Error = dialErr.Error()
		return result, nil
	}
	result.Success = true
	return result, nil
}

// Save 创建或更新主机。
func (h *HostService) Save(host config.Host) error {
	if err := config.ValidateHost(host); err != nil {
		return err
	}
	if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.Host) == "" || strings.TrimSpace(host.User) == "" {
		return errors.New("host name, address and user are required")
	}
	switch host.Auth.Method {
	case "", config.AuthMethodPassword:
	case config.AuthMethodKey:
		if strings.TrimSpace(host.Auth.KeyPath) == "" {
			return errors.New("key authentication requires a key path")
		}
	case config.AuthMethodAgent:
	default:
		return fmt.Errorf("unknown auth method %q", host.Auth.Method)
	}
	if host.Port == 0 {
		host.Port = 22
	}
	if host.Port < 1 || host.Port > 65535 {
		return errors.New("host port must be between 1 and 65535")
	}
	h.s.tmu.Lock()
	changed := false
	defer func() {
		h.s.tmu.Unlock()
		if changed {
			h.s.emitConfigChanged()
		}
	}()
	settings, err := h.s.Store.Load()
	if err != nil {
		return err
	}
	if host.ID == "" {
		host.ID = config.NewID("host")
		if err := validateJumpHosts(settings, host); err != nil {
			return err
		}
		settings.Hosts = append(settings.Hosts, host)
	} else {
		found := false
		for i := range settings.Hosts {
			if settings.Hosts[i].ID == host.ID {
				settings.Hosts[i] = host
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("host %s not found", host.ID)
		}
		if err := validateJumpHosts(settings, host); err != nil {
			return err
		}
	}
	err = h.s.Store.Save(settings)
	changed = err == nil
	return err
}

// Delete 删除主机（引用它的隧道一并删除）。
func (h *HostService) Delete(id string) error {
	// Stop matching runtime tunnels before removing their configuration. This
	// prevents an orphaned listener from surviving a host deletion.
	h.s.tmu.Lock()
	changed := false
	defer func() {
		h.s.tmu.Unlock()
		if changed {
			h.s.emitConfigChanged()
		}
	}()
	settings, err := h.s.Store.Load()
	if err != nil {
		return err
	}
	deleted := false
	for _, host := range settings.Hosts {
		if host.ID == id {
			deleted = true
			break
		}
	}
	if !deleted {
		return fmt.Errorf("host %s not found", id)
	}
	var tunnelIDs []string
	for _, tun := range settings.Tunnels {
		if tun.HostID == id {
			tunnelIDs = append(tunnelIDs, tun.ID)
		}
	}
	for _, tunnelID := range tunnelIDs {
		if err := h.s.Sup.Stop(tunnelID); err != nil && !errors.Is(err, supervisor.ErrNotRunning) {
			return err
		}
	}

	settings, err = h.s.Store.Load()
	if err != nil {
		return err
	}
	hosts := settings.Hosts[:0]
	tunnels := settings.Tunnels[:0]
	for _, host := range settings.Hosts {
		if host.ID != id {
			var cleanedJumps []string
			for _, jid := range host.JumpHostIDs {
				if jid != id {
					cleanedJumps = append(cleanedJumps, jid)
				}
			}
			host.JumpHostIDs = cleanedJumps
			hosts = append(hosts, host)
		}
	}
	for _, tun := range settings.Tunnels {
		if tun.HostID != id {
			tunnels = append(tunnels, tun)
		}
	}
	settings.Hosts = hosts
	settings.Tunnels = tunnels
	if err := h.s.Store.Save(settings); err != nil {
		return err
	}
	for _, tunnelID := range tunnelIDs {
		h.s.Stats.Delete(tunnelID)
	}
	changed = true
	return nil
}

func validPort(port int) bool { return port > 0 && port <= 65535 }

// validateJumpHosts rejects dangling, self-referential, and cyclic ProxyJump
// references at save time rather than waiting until a tunnel starts.
func validateJumpHosts(settings *config.Settings, candidate config.Host) error {
	known := make(map[string]config.Host, len(settings.Hosts)+1)
	for _, h := range settings.Hosts {
		known[h.ID] = h
	}
	known[candidate.ID] = candidate
	var visit func(string, map[string]bool) error
	visit = func(id string, path map[string]bool) error {
		if path[id] {
			return errors.New("host jump chain contains a cycle")
		}
		h, ok := known[id]
		if !ok {
			return fmt.Errorf("jump host %s not found", id)
		}
		path[id] = true
		for _, next := range h.JumpHostIDs {
			if next == "" {
				continue
			}
			if err := visit(next, path); err != nil {
				return err
			}
		}
		delete(path, id)
		return nil
	}
	return visit(candidate.ID, map[string]bool{})
}

// ---------- SettingsService ----------

// AppInfo 返回应用信息（前端关于页/调试用）。
type AppInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	ConfigPath string `json:"configPath"`
}

// Get 返回应用信息。
func (st *SettingsService) Get() (*AppInfo, error) {
	return &AppInfo{
		Name:       version.Name,
		Version:    version.Current(),
		ConfigPath: st.s.Store.Path(),
	}, nil
}

// SetEmitter 装配事件转发函数（在 application 创建后调用）。
func (s *Services) SetEmitter(e Emitter) {
	s.emu.Lock()
	defer s.emu.Unlock()
	s.emit = e
}

// SetExportChooser connects the native save dialog without coupling core to UI.
func (s *Services) SetExportChooser(choose func() (string, error)) {
	s.emu.Lock()
	defer s.emu.Unlock()
	s.chooseExport = choose
}

// 公开构造，供 main 传递给 Wails 绑定。
func (s *Services) TunnelService() *TunnelService     { return s.tunnel() }
func (s *Services) HostService() *HostService         { return s.host() }
func (s *Services) SettingsService() *SettingsService { return s.settings() }
