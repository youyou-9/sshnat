package forward_test

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/sshnat/sshnat/core/forward"
	"github.com/sshnat/sshnat/core/ssh"
	"github.com/sshnat/sshnat/core/stats"
)

// mockSSHServer 是一个进程内 SSH 服务器：接受密码认证，
// 支持 direct-tcpip 通道（即 -L 转发在服务器侧行为的最小实现）。
type mockSSHServer struct {
	ln       net.Listener
	target   string // direct-tcpip 允许的目标地址
	password string
}

func startMockSSHServer(t *testing.T, target, password string) *mockSSHServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &mockSSHServer{ln: ln, target: target, password: password}
	go s.serve(t)
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *mockSSHServer) addr() string { return s.ln.Addr().String() }

func (s *mockSSHServer) serve(t *testing.T) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn, t)
	}
}

func (s *mockSSHServer) handle(conn net.Conn, t *testing.T) {
	defer conn.Close()
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(meta gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			if meta.User() == "testuser" && string(password) == s.password {
				return nil, nil
			}
			return nil, fmt.Errorf("auth rejected")
		},
	}
	// 测试用临时 host key（运行时生成）。
	signer, err := testSigner()
	if err != nil {
		t.Logf("hostkey: %v", err)
		return
	}
	cfg.AddHostKey(signer)

	sconn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	go gossh.DiscardRequests(reqs)

	for ch := range chans {
		switch ch.ChannelType() {
		case "direct-tcpip":
			var payload struct {
				Addr     string
				Port     uint32
				Orig     string
				OrigPort uint32
			}
			if err := gossh.Unmarshal(ch.ExtraData(), &payload); err != nil {
				ch.Reject(gossh.ConnectionFailed, "bad payload")
				continue
			}
			if payload.Addr != s.target {
				ch.Reject(gossh.Prohibited, "target not allowed")
				continue
			}
			up, err := net.Dial("tcp", net.JoinHostPort(payload.Addr, fmt.Sprint(payload.Port)))
			if err != nil {
				ch.Reject(gossh.ConnectionFailed, err.Error())
				continue
			}
			channel, reqs, err := ch.Accept()
			if err != nil {
				up.Close()
				continue
			}
			go gossh.DiscardRequests(reqs)
			go func() {
				defer up.Close()
				go io.Copy(up, channel)
				io.Copy(channel, up)
			}()
		case "session":
			// keepalive 等通过全局请求处理，session 一律拒绝。
			ch.Reject(gossh.UnknownChannelType, "no sessions")
		default:
			ch.Reject(gossh.UnknownChannelType, "unsupported")
		}
	}
	_ = sconn
}

var (
	testSignerOnce sync.Once
	testSignerVal  gossh.Signer
	testSignerErr  error
)

// testSigner 运行时生成一次 ed25519 host key。
func testSigner() (gossh.Signer, error) {
	testSignerOnce.Do(func() {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			testSignerErr = err
			return
		}
		testSignerVal, testSignerErr = gossh.NewSignerFromKey(priv)
	})
	return testSignerVal, testSignerErr
}

// startEchoServer 启动回显服务器，返回其地址。
func startEchoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn) // 回显
			}()
		}
	}()
	return ln.Addr().String()
}

// pickFreePort 选一个空闲 TCP 端口供本地监听使用。
func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestLocalForwardEndToEnd 覆盖验收主链路：
// 客户端 → 本地监听 → SSH(direct-tcpip) → 目标回显服务器。
func TestLocalForwardEndToEnd(t *testing.T) {
	target := startEchoServer(t)
	targetHost, targetPortStr, _ := net.SplitHostPort(target)

	// 服务器侧 unix socket 目标（direct-streamlocal）暂不在 mock 内实现，
	// 单独覆盖 TCP 目标。
	sshSrv := startMockSSHServer(t, targetHost, "secret")

	reg := stats.NewRegistry()
	counter := reg.Get("t1")

	client, err := ssh.Dial(t.Context(), ssh.DialOptions{
		Host:                hostOf(sshSrv.addr()),
		Port:                portOf(sshSrv.addr()),
		User:                "testuser",
		Auth:                ssh.AuthConfig{Type: ssh.AuthPassword, Password: "secret"},
		KnownHostsFile:      filepath.Join(t.TempDir(), "known_hosts"),
		InsecureSkipHostKey: false,
	})
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	localPort := pickFreePort(t)
	fw := forward.NewLocal(client, forward.LocalConfig{
		ListenNetwork: "tcp",
		ListenAddr:    fmt.Sprintf("127.0.0.1:%d", localPort),
		TargetNetwork: "tcp",
		TargetAddr:    net.JoinHostPort(targetHost, targetPortStr),
	}, counter)

	if err := fw.Start(); err != nil {
		t.Fatalf("forward start: %v", err)
	}
	defer fw.Stop()

	// 等监听就绪。
	var conn net.Conn
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 500*time.Millisecond)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("local forward never became reachable")
	}
	defer conn.Close()

	// 发送数据并期待回显。
	msg := "hello via sshnat -L\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && got == "" {
		t.Fatalf("read echo: %v", err)
	}
	if got[:len(msg)] != msg {
		t.Fatalf("echo mismatch: got %q", got)
	}

	// 流量计数应已增长。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := counter.Snapshot()
		if snap.Tx > 0 && snap.Rx > 0 && snap.TotalConn >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap := counter.Snapshot()
	if snap.Tx == 0 || snap.Rx == 0 || snap.TotalConn < 1 {
		t.Fatalf("stats not recorded: %+v", snap)
	}

	// 正常停止。
	if err := fw.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-fw.Done():
	case <-time.After(3 * time.Second):
		t.Fatalf("forwarder did not finish after Stop")
	}
	if err := fw.Err(); err != nil {
		t.Fatalf("Err after normal stop should be nil, got %v", err)
	}
}

// TestDialBadPassword 确认认证失败会返回错误而非挂起。
func TestDialBadPassword(t *testing.T) {
	target := startEchoServer(t)
	sshSrv := startMockSSHServer(t, hostOf(target), "secret")

	_, err := ssh.Dial(t.Context(), ssh.DialOptions{
		Host:                hostOf(sshSrv.addr()),
		Port:                portOf(sshSrv.addr()),
		User:                "testuser",
		Auth:                ssh.AuthConfig{Type: ssh.AuthPassword, Password: "wrong"},
		InsecureSkipHostKey: true,
	})
	if err == nil {
		t.Fatalf("expected auth error")
	}
}

func hostOf(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	return h
}

func portOf(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	var port int
	fmt.Sscanf(p, "%d", &port)
	return port
}
