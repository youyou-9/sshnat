// Package ssh 封装 golang.org/x/crypto/ssh 的连接管理：
// 密码/私钥/keyboard-interactive/agent 认证、hostkey 校验（accept-new）、
// 多级跳板链（ProxyJump）、keepalive 与防泄漏的生命周期管理。
//
// 本包是纯 Go 库，不依赖任何 UI 框架。
package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// AuthType 认证方式。
type AuthType string

const (
	AuthPassword AuthType = "password" // 密码（含 keyboard-interactive 回退）
	AuthKey      AuthType = "key"      // 私钥文件，可带口令
	AuthAgent    AuthType = "agent"    // ssh-agent
)

// AuthConfig 描述一种认证方式。
type AuthConfig struct {
	Type          AuthType `json:"type"`
	Password      string   `json:"password,omitempty"`
	KeyPath       string   `json:"keyPath,omitempty"`
	KeyPassphrase string   `json:"keyPassphrase,omitempty"`
	AgentSocket   string   `json:"agentSocket,omitempty"` // 空 = SSH_AUTH_SOCK
}

// DialOptions 一次 SSH 拨号的全部参数。
type DialOptions struct {
	Host string
	Port int
	User string

	Auth AuthConfig

	// JumpHosts 为跳板链，按从近到远排列：JumpHosts[0] 直连，
	// JumpHosts[i] 通过 JumpHosts[i-1] 抵达，目标主机经最后一个跳板抵达。
	JumpHosts []DialOptions

	// KnownHostsFile 为 known_hosts 路径；文件不存在时新记录会写入该路径
	// （accept-new 语义）。为空时使用用户 SSH 目录下的默认文件。
	KnownHostsFile string
	// InsecureSkipHostKey 跳过 hostkey 校验（仅测试用）。
	InsecureSkipHostKey bool

	Timeout          time.Duration // TCP/握手超时，默认 15s
	Keepalive        time.Duration // keepalive 间隔，默认 15s；<=0 禁用
	KeepaliveTimeout time.Duration // 单次 keepalive 应答超时，默认 10s
}

func (o *DialOptions) addr() string {
	port := o.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(o.Host, fmt.Sprint(port))
}

func (o *DialOptions) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return 15 * time.Second
}

// Client 是一个受管 SSH 客户端：内建 keepalive 探测与关闭广播。
type Client struct {
	sshRawClient

	opts   DialOptions
	closed chan struct{}
	once   closeOnce
}

// Dial 建立SSH 连接（含认证与跳板链）。
func Dial(ctx context.Context, opts DialOptions) (*Client, error) {
	clientConfig, err := newClientConfig(&opts)
	if err != nil {
		return nil, err
	}

	raw, err := dialChain(ctx, &opts, clientConfig)
	if err != nil {
		return nil, err
	}

	c := &Client{
		sshRawClient: raw,
		opts:         opts,
		closed:       make(chan struct{}),
	}

	ka := opts.Keepalive
	if ka > 0 {
		kaTimeout := opts.KeepaliveTimeout
		if kaTimeout <= 0 {
			kaTimeout = 10 * time.Second
		}
		go c.keepaliveLoop(ka, kaTimeout)
	}

	return c, nil
}

// dialChain 处理跳板链：逐级拨号，最终在最后一级上建立到目标的 SSH 会话。
func dialChain(ctx context.Context, opts *DialOptions, cfg *sshClientConfig) (sshRawClient, error) {
	targetAddr := opts.addr()

	if len(opts.JumpHosts) == 0 {
		d := net.Dialer{Timeout: opts.timeout()}
		conn, err := d.DialContext(ctx, "tcp", targetAddr)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", targetAddr, err)
		}
		conn.SetDeadline(time.Now().Add(opts.timeout()))
		cli, err := newClientConn(ctx, conn, targetAddr, cfg)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("handshake %s: %w", targetAddr, err)
		}
		return cli, nil
	}

	// 递归拨第一级跳板。Dial on the first jump resolves any chain configured
	// on that host; the loop below then walks the remaining hops explicitly.
	jump := opts.JumpHosts[0]
	jumpClient, err := Dial(ctx, jump)
	if err != nil {
		return nil, fmt.Errorf("jump %s@%s: %w", jump.User, jump.addr(), err)
	}

	current := sshRawClient(jumpClient)
	var upstream io.Closer = jumpClient
	for _, hop := range opts.JumpHosts[1:] {
		conn, err := dialRawContext(ctx, current, hop.addr())
		if err != nil {
			_ = upstream.Close()
			return nil, fmt.Errorf("jump->%s: %w", hop.addr(), err)
		}
		conn.SetDeadline(time.Now().Add(hop.timeout()))
		hopCfg, err := newClientConfig(&hop)
		if err != nil {
			_ = conn.Close()
			_ = upstream.Close()
			return nil, err
		}
		cli, err := newClientConn(ctx, conn, hop.addr(), hopCfg)
		if err != nil {
			_ = conn.Close()
			_ = upstream.Close()
			return nil, fmt.Errorf("handshake via jump ->%s: %w", hop.addr(), err)
		}
		wrapped := &chainedClient{sshClient: cli, upstream: upstream}
		current = wrapped
		upstream = wrapped
	}

	conn, err := dialRawContext(ctx, current, targetAddr)
	if err != nil {
		_ = upstream.Close()
		return nil, fmt.Errorf("jump->%s: %w", targetAddr, err)
	}
	conn.SetDeadline(time.Now().Add(opts.timeout()))
	cli, err := newClientConn(ctx, conn, targetAddr, cfg)
	if err != nil {
		_ = conn.Close()
		_ = upstream.Close()
		return nil, fmt.Errorf("handshake via jump ->%s: %w", targetAddr, err)
	}
	return &chainedClient{sshClient: cli, upstream: upstream}, nil
}

// chainedClient 关闭时联动关闭上游跳板连接。
type chainedClient struct {
	*sshClient
	upstream io.Closer
}

func (c *chainedClient) Close() error {
	err := c.sshClient.Close()
	_ = c.upstream.Close()
	return err
}

// keepaliveLoop 定期发送 keepalive，失败即关闭连接并广播断开。
func (c *Client) keepaliveLoop(interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
		}
		done := make(chan error, 1)
		go func() {
			_, _, err := c.sshRawClient.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()
		select {
		case <-c.closed:
			return
		case err := <-done:
			if err != nil {
				// 连接已死：关闭以唤醒所有等待者。
				_ = c.Close()
				return
			}
		case <-time.After(timeout):
			_ = c.Close()
			return
		}
	}
}

// Done 返回一个在连接关闭（主动或故障）时被关闭的 channel。
func (c *Client) Done() <-chan struct{} { return c.closed }

// Close 关闭连接。
func (c *Client) Close() error {
	c.once.Do(func() {
		close(c.closed)
	})
	return c.sshRawClient.Close()
}
