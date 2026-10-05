// Package config 定义 SSHNat 的配置模型、JSON 持久化，
// 以及 OpenSSH 命令行（如 `ssh -L 8080:db:3306 user@host`）解析器。
package config

// 转发类型。
const (
	TypeLocal   = "L" // -L 本地转发
	TypeRemote  = "R" // -R 远程转发
	TypeDynamic = "D" // -D 动态转发（SOCKS）
)

// 认证方式。
const (
	AuthMethodPassword = "password"
	AuthMethodKey      = "key"
	AuthMethodAgent    = "agent"
)

// Host key verification policies. Empty values retain accept-new behavior.
const (
	HostKeyAcceptNew = "accept-new"
	HostKeyStrict    = "strict"
)

// Host 是一台 SSH 服务器（含认证与跳板配置）。
type Host struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`

	Auth AuthConfig `json:"auth"`

	// JumpHostIDs 为跳板链，按从近到远排列。
	JumpHostIDs []string `json:"jumpHostIds,omitempty"`

	KeepaliveSeconds      int    `json:"keepaliveSeconds,omitempty"` // 0 = 默认 15s
	KnownHostsFile        string `json:"knownHostsFile,omitempty"`
	HostKeyPolicy         string `json:"hostKeyPolicy,omitempty"`         // accept-new | strict
	ConnectTimeoutSeconds int    `json:"connectTimeoutSeconds,omitempty"` // 0 = 默认 15s
}

// AuthConfig 认证配置。
type AuthConfig struct {
	Method        string `json:"method"` // password | key | agent
	Password      string `json:"password,omitempty"`
	KeyPath       string `json:"keyPath,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
	AgentSocket   string `json:"agentSocket,omitempty"`
}

// Tunnel 是一条端口转发规则。
type Tunnel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	HostID string `json:"hostId"` // 使用的 SSH 主机

	Type string `json:"type"` // L | R | D

	// -L：本地监听 → 经 SSH 到目标。
	LocalBindHost string `json:"localBindHost,omitempty"` // 默认 127.0.0.1
	LocalPort     int    `json:"localPort,omitempty"`
	LocalSocket   string `json:"localSocket,omitempty"` // 本地 unix socket（与 LocalPort 二选一）

	TargetHost   string `json:"targetHost,omitempty"`   // -L 目标主机
	TargetPort   int    `json:"targetPort,omitempty"`   // -L 目标端口
	TargetSocket string `json:"targetSocket,omitempty"` // 服务器侧 unix socket（与 TargetHost/Port 二选一）

	// -R：服务器侧监听 → 经 SSH 回本地目标。
	RemoteBindHost string `json:"remoteBindHost,omitempty"`
	RemotePort     int    `json:"remotePort,omitempty"`
	RemoteSocket   string `json:"remoteSocket,omitempty"`

	// -D：SOCKS 监听端口。
	SocksPort int `json:"socksPort,omitempty"`

	AutoStart bool `json:"autoStart"`
}

// Settings 是完整配置文档。
type Settings struct {
	Version int      `json:"version"`
	Hosts   []Host   `json:"hosts"`
	Tunnels []Tunnel `json:"tunnels"`
}

// FindHost 按 ID 查找主机。
func (s *Settings) FindHost(id string) (*Host, bool) {
	for i := range s.Hosts {
		if s.Hosts[i].ID == id {
			return &s.Hosts[i], true
		}
	}
	return nil, false
}

// FindTunnel 按 ID 查找隧道。
func (s *Settings) FindTunnel(id string) (*Tunnel, bool) {
	for i := range s.Tunnels {
		if s.Tunnels[i].ID == id {
			return &s.Tunnels[i], true
		}
	}
	return nil, false
}
