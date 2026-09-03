package supervisor_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

// TestSupervisorReconnectsAndStops 验证：
//  1. 指向不可达 SSH 的隧道会进入 reconnecting 并按指数退避重试；
//  2. Stop 能立即中断退避等待并收敛到 stopped；
//  3. 状态事件通过事件总线推送。
func TestSupervisorReconnectsAndStops(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// 占用端口但不提供 SSH 服务，制造连接拒绝。
	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer deadLn.Close()
	addr := deadLn.Addr().(*net.TCPAddr)
	// 立即断开所有入站连接：TCP 能连上但 SSH 握手立刻失败。
	go func() {
		for {
			c, err := deadLn.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	settings := &config.Settings{
		Version: 1,
		Hosts: []config.Host{
			{ID: "host-1", Name: "dead", Host: "127.0.0.1", Port: addr.Port, User: "u",
				Auth: config.AuthConfig{Method: "password", Password: "x"}},
		},
		Tunnels: []config.Tunnel{
			{ID: "tnl-1", Name: "t1", HostID: "host-1", Type: "L",
				LocalPort: 59999, TargetHost: "127.0.0.1", TargetPort: 80},
		},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	store := config.NewStore(cfgPath)
	reg := stats.NewRegistry()
	sup := supervisor.New(store, reg)

	ch, cancel := sup.Subscribe()
	defer cancel()

	if err := sup.Start("tnl-1"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 等待第一次连接失败进入重连（baseBackoff=1s 内应有 starting→失败事件）。
	sawStarting := false
	sawReconnecting := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !(sawStarting && sawReconnecting) {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Type != supervisor.EventStatus || ev.TunnelID != "tnl-1" {
				continue
			}
			switch ev.Status {
			case supervisor.StatusStarting:
				sawStarting = true
			case supervisor.StatusReconnecting:
				sawReconnecting = true
			}
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !sawStarting || !sawReconnecting {
		t.Fatalf("expected starting+reconnecting, got starting=%v reconnecting=%v", sawStarting, sawReconnecting)
	}

	// Stop 应在退避等待中立即收敛。
	stopped := make(chan error, 1)
	go func() { stopped <- sup.Stop("tnl-1") }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Stop did not return in time")
	}

	if st := sup.Status("tnl-1"); st != supervisor.StatusStopped {
		t.Fatalf("status after stop = %q, want stopped", st)
	}

	// 再次 Stop 返回 ErrNotRunning。
	if err := sup.Stop("tnl-1"); err == nil {
		t.Fatalf("second stop should fail")
	}
}

func TestStatsTickerAcceptsNonPositiveInterval(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	sup := supervisor.New(store, stats.NewRegistry())
	stop := sup.StartStatsTicker(0)
	stop()
	stop()
}
