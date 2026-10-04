package forward_test

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
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

// mockRemoteSSHServer 模拟支持 tcpip-forward 和 forwarded-tcpip 的 SSH 服务端。
type mockRemoteSSHServer struct {
	ln           net.Listener
	password     string
	ignoreCancel bool
	ignoreListen bool
	listenSeen   chan struct{}

	mu        sync.Mutex
	listeners map[string]net.Listener
}

func startMockRemoteSSHServer(t *testing.T, password string) *mockRemoteSSHServer {
	return startMockRemoteSSHServerWithCancel(t, password, false)
}

func startMockRemoteSSHServerWithCancel(t *testing.T, password string, ignoreCancel bool) *mockRemoteSSHServer {
	return startMockRemoteSSHServerWithBehavior(t, password, ignoreCancel, false)
}

func startMockRemoteSSHServerWithBehavior(t *testing.T, password string, ignoreCancel, ignoreListen bool) *mockRemoteSSHServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &mockRemoteSSHServer{
		ln:           ln,
		password:     password,
		ignoreCancel: ignoreCancel,
		ignoreListen: ignoreListen,
		listenSeen:   make(chan struct{}),
		listeners:    make(map[string]net.Listener),
	}
	go s.serve(t)
	t.Cleanup(func() {
		ln.Close()
		s.mu.Lock()
		for _, l := range s.listeners {
			l.Close()
		}
		s.mu.Unlock()
	})
	return s
}

func (s *mockRemoteSSHServer) addr() string { return s.ln.Addr().String() }

func (s *mockRemoteSSHServer) serve(t *testing.T) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn, t)
	}
}

func (s *mockRemoteSSHServer) handle(conn net.Conn, t *testing.T) {
	defer conn.Close()
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(meta gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			if meta.User() == "testuser" && string(password) == s.password {
				return nil, nil
			}
			return nil, fmt.Errorf("auth rejected")
		},
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return
	}
	cfg.AddHostKey(signer)

	sconn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()

	go func() {
		for ch := range chans {
			ch.Reject(gossh.UnknownChannelType, "unsupported")
		}
	}()

	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			if s.ignoreListen {
				close(s.listenSeen)
				continue
			}
			var payload struct {
				Addr string
				Port uint32
			}
			if err := gossh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			boundLn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", payload.Port))
			if err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			boundPort := uint32(boundLn.Addr().(*net.TCPAddr).Port)
			s.mu.Lock()
			s.listeners[fmt.Sprintf("127.0.0.1:%d", boundPort)] = boundLn
			s.mu.Unlock()

			replyPayload := gossh.Marshal(struct{ Port uint32 }{boundPort})
			_ = req.Reply(true, replyPayload)

			// 在服务端接受外部连接并转发回客户端的 forwarded-tcpip 通道
			go func(l net.Listener, port uint32) {
				for {
					c, err := l.Accept()
					if err != nil {
						return
					}
					go func(in net.Conn) {
						defer in.Close()
						chPayload := gossh.Marshal(struct {
							DestAddr   string
							DestPort   uint32
							OriginAddr string
							OriginPort uint32
						}{
							DestAddr:   "127.0.0.1",
							DestPort:   port,
							OriginAddr: "127.0.0.1",
							OriginPort: 12345,
						})
						channel, reqs, err := sconn.OpenChannel("forwarded-tcpip", chPayload)
						if err != nil {
							return
						}
						go gossh.DiscardRequests(reqs)
						var wg sync.WaitGroup
						wg.Add(2)
						go func() {
							defer wg.Done()
							defer channel.Close()
							buf := make([]byte, 4096)
							for {
								n, rerr := in.Read(buf)
								if n > 0 {
									_, _ = channel.Write(buf[:n])
								}
								if rerr != nil {
									break
								}
							}
						}()
						go func() {
							defer wg.Done()
							defer in.Close()
							buf := make([]byte, 4096)
							for {
								n, rerr := channel.Read(buf)
								if n > 0 {
									_, _ = in.Write(buf[:n])
								}
								if rerr != nil {
									break
								}
							}
						}()
						wg.Wait()
					}(c)
				}
			}(boundLn, boundPort)

		case "cancel-tcpip-forward":
			if !s.ignoreCancel {
				_ = req.Reply(true, nil)
			}
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// TestRemoteForwardEndToEnd 验证 -R 远程转发：
// 外部客户端 → SSH 服务端监听端口 → SSH (forwarded-tcpip) → 本地目标回显服务。
func TestRemoteForwardEndToEnd(t *testing.T) {
	// 启动本地目标服务（回显服务器）
	target := startEchoServer(t)
	_, targetPortStr, _ := net.SplitHostPort(target)
	var targetPort int
	fmt.Sscanf(targetPortStr, "%d", &targetPort)

	// 启动 mock SSH 服务端
	sshSrv := startMockRemoteSSHServer(t, "secret")

	reg := stats.NewRegistry()
	counter := reg.Get("remote-1")

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

	remotePort := pickFreePort(t)
	fw := forward.NewRemote(client, forward.RemoteConfig{
		BindHost:      "127.0.0.1",
		BindPort:      remotePort,
		TargetNetwork: "tcp",
		TargetAddr:    fmt.Sprintf("127.0.0.1:%d", targetPort),
	}, counter)

	if err := fw.Start(); err != nil {
		t.Fatalf("remote forward start: %v", err)
	}
	defer fw.Stop()

	// 连入远程服务端的监听端口
	var conn net.Conn
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort), 200*time.Millisecond)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("remote forward port never reachable")
	}
	defer conn.Close()

	msg := "hello via sshnat -R\n"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write to remote port: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if got != msg {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}

	// 验证流量计数
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := counter.Snapshot()
		if snap.Tx > 0 && snap.Rx > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap := counter.Snapshot()
	if snap.Tx == 0 || snap.Rx == 0 {
		t.Fatalf("stats should be recorded: %+v", snap)
	}

	if err := fw.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestRemoteStopInterruptsUnansweredCancellation(t *testing.T) {
	srv := startMockRemoteSSHServerWithCancel(t, "secret", true)
	client, err := ssh.Dial(t.Context(), ssh.DialOptions{
		Host: hostOf(srv.addr()), Port: portOf(srv.addr()), User: "testuser",
		Auth:           ssh.AuthConfig{Type: ssh.AuthPassword, Password: "secret"},
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	fw := forward.NewRemote(client, forward.RemoteConfig{BindHost: "127.0.0.1", BindPort: pickFreePort(t), TargetAddr: "127.0.0.1:1"}, stats.NewRegistry().Get("cancel"))
	if err := fw.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- fw.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		client.Close()
		t.Fatal("Remote.Stop hung waiting for cancellation reply")
	}
	select {
	case <-fw.Done():
	default:
		t.Fatal("Remote.Stop returned before workers finished")
	}
}

func TestRemoteStopDuringUnansweredStartup(t *testing.T) {
	srv := startMockRemoteSSHServerWithBehavior(t, "secret", false, true)
	client, err := ssh.Dial(t.Context(), ssh.DialOptions{
		Host: hostOf(srv.addr()), Port: portOf(srv.addr()), User: "testuser",
		Auth:           ssh.AuthConfig{Type: ssh.AuthPassword, Password: "secret"},
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	fw := forward.NewRemote(client, forward.RemoteConfig{BindHost: "127.0.0.1", BindPort: pickFreePort(t), TargetAddr: "127.0.0.1:1"}, stats.NewRegistry().Get("startup"))
	started := make(chan error, 1)
	go func() { started <- fw.Start() }()
	select {
	case <-srv.listenSeen:
	case <-time.After(time.Second):
		client.Close()
		t.Fatal("remote listen request was never sent")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- fw.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		client.Close()
		t.Fatal("Stop could not interrupt pending remote Start")
	}
	if err := <-started; err == nil {
		t.Fatal("Start unexpectedly succeeded after Stop")
	}
}
