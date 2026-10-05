package ssh

import (
	"context"
	"net"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// 对 golang.org/x/crypto/ssh 的薄别名层，便于包内统一引用与未来替换实现。

type (
	sshClient       = gossh.Client
	sshClientConfig = gossh.ClientConfig
	closeOnce       = sync.Once
)

// sshRawClient 是最终客户端需要的最小能力集：
// *gossh.Client 与带跳板联动的 chainedClient 都满足。
type sshRawClient interface {
	gossh.Conn
	Dial(network, addr string) (net.Conn, error)
	Listen(network, addr string) (net.Listener, error)
	ListenUnix(socketPath string) (net.Listener, error)
	OpenChannel(name string, data []byte) (gossh.Channel, <-chan *gossh.Request, error)
}

// ListenUnix requests an OpenSSH stream-local forward on the server. It is
// kept on the wrapper so forward.Remote can support -R Unix socket listeners
// through both direct and jump-host connections.
func (c *Client) ListenUnix(socketPath string) (net.Listener, error) {
	return c.sshRawClient.ListenUnix(socketPath)
}

// newClientConn 握手并建立客户端（支持 context 取消）。
// （gossh.NewClientConn 返回内部 connection，需用 NewClient 包装。）
func newClientConn(ctx context.Context, conn net.Conn, addr string, cfg *sshClientConfig) (*sshClient, error) {
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()
	c, chans, reqs, err := gossh.NewClientConn(conn, addr, cfg)
	close(handshakeDone)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	// The deadline only covers TCP connect and SSH handshake. Keeping it set
	// would make an otherwise healthy idle session expire after the dial timeout.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = c.Close()
		return nil, err
	}
	return gossh.NewClient(c, chans, reqs), nil
}

func dialRawContext(ctx context.Context, raw sshRawClient, addr string) (net.Conn, error) {
	return dialConnContext(ctx, func() (net.Conn, error) { return raw.Dial("tcp", addr) })
}

func dialConnContext(ctx context.Context, dial func() (net.Conn, error)) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := dial()
		ch <- result{conn, err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
}

// DialContext 通过 SSH 客户端拨号（TCP 或 Unix socket），支持 ctx 取消与超时。
func (c *Client) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network == "unix" {
		return dialConnContext(ctx, func() (net.Conn, error) { return c.DialUnix(addr) })
	}
	return dialRawContext(ctx, c.sshRawClient, addr)
}

// netDialContext 通过 SSH 客户端拨号 TCP，支持 ctx 取消。
func (c *Client) netDialContext(ctx context.Context, addr string) (net.Conn, error) {
	return dialRawContext(ctx, c.sshRawClient, addr)
}

// DialUnix 通过 direct-streamlocal@openssh.com 通道连接服务器侧的 Unix socket。
// OpenSSH 的 client.Dial 只支持 TCP，这里补齐 Unix socket 目标。
func (c *Client) DialUnix(path string) (net.Conn, error) {
	payload := gossh.Marshal(struct {
		SocketPath string
		Reserved1  string
		Reserved2  uint32
	}{path, "", 0})
	ch, reqs, err := c.sshRawClient.OpenChannel("direct-streamlocal@openssh.com", payload) //nolint:noctx // 由上层 accept 循环管理生命周期
	if err != nil {
		return nil, err
	}
	go func() {
		for req := range reqs {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}()
	return &channelConn{Channel: ch}, nil
}

// channelConn 将 ssh.Channel 适配为 net.Conn。
type channelConn struct {
	gossh.Channel
}

func (c *channelConn) LocalAddr() net.Addr  { return chanAddr{} }
func (c *channelConn) RemoteAddr() net.Addr { return chanAddr{} }
func (c *channelConn) SetDeadline(time.Time) error {
	return nil // SSH 通道不支持超时设置，静默接受
}
func (c *channelConn) SetReadDeadline(time.Time) error  { return nil }
func (c *channelConn) SetWriteDeadline(time.Time) error { return nil }

type chanAddr struct{}

func (chanAddr) Network() string { return "ssh-channel" }
func (chanAddr) String() string  { return "ssh-channel" }
