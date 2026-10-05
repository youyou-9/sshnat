package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

// startPasswordSSHServer starts the smallest SSH server needed to verify the
// HostService connectivity probe: password authentication and no channels.
func startPasswordSSHServer(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen SSH test server: %v", err)
	}
	signer, err := newTestHostSigner()
	if err != nil {
		listener.Close()
		t.Fatalf("generate SSH host key: %v", err)
	}
	serverConfig := &gossh.ServerConfig{
		PasswordCallback: func(meta gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			if meta.User() == "testuser" && string(password) == "secret" {
				return nil, nil
			}
			return nil, fmt.Errorf("authentication rejected")
		},
	}
	serverConfig.AddHostKey(signer)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, channels, requests, handshakeErr := gossh.NewServerConn(conn, serverConfig)
				if handshakeErr != nil {
					return
				}
				go gossh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(gossh.Prohibited, "test server does not open channels")
				}
			}()
		}
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		t.Fatalf("split SSH test address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		listener.Close()
		t.Fatalf("parse SSH test port: %v", err)
	}
	return host, port
}

func newTestHostSigner() (gossh.Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return gossh.NewSignerFromKey(privateKey)
}

func setupTestServices(t *testing.T) *Services {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	store := config.NewStore(cfgPath)
	reg := stats.NewRegistry()
	sup := supervisor.New(store, reg)
	return NewServices(store, sup, reg)
}

func TestHostServiceCRUDAndCascadeDelete(t *testing.T) {
	services := setupTestServices(t)
	hostSvc := services.HostService()
	tunnelSvc := services.TunnelService()

	// 1. 创建主机
	host := config.Host{
		Name: "bastion",
		Host: "bastion.example.com",
		Port: 22,
		User: "root",
		Auth: config.AuthConfig{Method: "password", Password: "secret"},
	}
	if err := hostSvc.Save(host); err != nil {
		t.Fatalf("save host: %v", err)
	}

	hosts, err := hostSvc.List()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", hosts)
	}
	hostID := hosts[0].ID

	// 2. 创建关联该主机的隧道
	tun, err := tunnelSvc.Create(CreateTunnelRequest{
		Name:       "mysql-tunnel",
		HostID:     hostID,
		Type:       config.TypeLocal,
		LocalPort:  3306,
		TargetHost: "127.0.0.1",
		TargetPort: 3306,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	if tun.HostID != hostID {
		t.Fatalf("tunnel host ID mismatch: got %s want %s", tun.HostID, hostID)
	}

	// 3. 删除主机并验证级联删除隧道
	if err := hostSvc.Delete(hostID); err != nil {
		t.Fatalf("delete host: %v", err)
	}

	hostsAfter, err := hostSvc.List()
	if err != nil || len(hostsAfter) != 0 {
		t.Fatalf("host should be deleted: %v", hostsAfter)
	}

	tunnelsAfter, err := tunnelSvc.List()
	if err != nil || len(tunnelsAfter) != 0 {
		t.Fatalf("associated tunnels should be cascade-deleted: %v", tunnelsAfter)
	}
}

func TestTunnelServiceCreateFromSSHCommand(t *testing.T) {
	services := setupTestServices(t)
	tunnelSvc := services.TunnelService()
	hostSvc := services.HostService()

	cmd := "ssh -L 8080:web.internal:80 -D 1080 alice@jump.example.com"
	tunnels, err := tunnelSvc.CreateFromSSHCommand(cmd)
	if err != nil {
		t.Fatalf("create from ssh cmd: %v", err)
	}
	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}

	hosts, err := hostSvc.List()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("expected 1 host created, got %d", len(hosts))
	}
	if hosts[0].User != "alice" || hosts[0].Host != "jump.example.com" {
		t.Fatalf("unexpected host: %+v", hosts[0])
	}
}

func TestTunnelServiceCreateFromSSHCommandPreservesJumps(t *testing.T) {
	services := setupTestServices(t)
	tunnelSvc := services.TunnelService()
	if _, err := tunnelSvc.CreateFromSSHCommand("ssh -J jump-user@jump.example:2200 root@target.example -L 8080:db:80"); err != nil {
		t.Fatalf("create from ssh command: %v", err)
	}
	hosts, err := services.HostService().List()
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected target and jump hosts, got %d", len(hosts))
	}
	var target, jump *config.Host
	for i := range hosts {
		switch hosts[i].Host {
		case "target.example":
			target = &hosts[i]
		case "jump.example":
			jump = &hosts[i]
		}
	}
	if target == nil || jump == nil || len(target.JumpHostIDs) != 1 || target.JumpHostIDs[0] != jump.ID {
		t.Fatalf("jump topology was not persisted: hosts=%+v", hosts)
	}
	tunnels, err := tunnelSvc.List()
	if err != nil || len(tunnels) != 1 || tunnels[0].HostID != target.ID {
		t.Fatalf("tunnel host reference incorrect: tunnels=%+v err=%v", tunnels, err)
	}
	if _, err := tunnelSvc.CreateFromSSHCommand("ssh root@target.example -L 8081:db:81"); err != nil {
		t.Fatalf("re-import command: %v", err)
	}
	hostsAfter, _ := services.HostService().List()
	if len(hostsAfter) != 2 {
		t.Fatalf("re-import should reuse target and jump hosts, got %d", len(hostsAfter))
	}
	tunnelsAfter, _ := tunnelSvc.List()
	if len(tunnelsAfter) != 2 || tunnelsAfter[1].HostID != target.ID {
		t.Fatalf("re-imported tunnel host reference incorrect: %+v", tunnelsAfter)
	}
}

func TestTunnelServiceUpdate(t *testing.T) {
	services := setupTestServices(t)
	hostSvc := services.HostService()
	tunnelSvc := services.TunnelService()

	host := config.Host{
		Name: "server",
		Host: "64.110.117.189",
		Port: 22,
		User: "root",
		Auth: config.AuthConfig{Method: "key", KeyPath: "~/.ssh/id_rsa"},
	}
	if err := hostSvc.Save(host); err != nil {
		t.Fatalf("save host: %v", err)
	}
	hosts, _ := hostSvc.List()
	hostID := hosts[0].ID

	// 1. 创建隧道
	created, err := tunnelSvc.Create(CreateTunnelRequest{
		Name:       "cpa",
		HostID:     hostID,
		Type:       config.TypeLocal,
		LocalPort:  8317,
		TargetHost: "64.110.117.189",
		TargetPort: 8317,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	// 2. 更新隧道目标为 127.0.0.1
	updated, err := tunnelSvc.Update(UpdateTunnelRequest{
		ID:         created.ID,
		Name:       "cpa-updated",
		HostID:     hostID,
		Type:       config.TypeLocal,
		LocalPort:  8317,
		TargetHost: "127.0.0.1",
		TargetPort: 8317,
	})
	if err != nil {
		t.Fatalf("update tunnel: %v", err)
	}
	if updated.Name != "cpa-updated" || updated.TargetHost != "127.0.0.1" {
		t.Fatalf("unexpected updated tunnel: %+v", updated)
	}

	// 3. 再次获取验证
	fetched, err := tunnelSvc.Get(created.ID)
	if err != nil {
		t.Fatalf("get tunnel: %v", err)
	}
	if fetched.Name != "cpa-updated" || fetched.TargetHost != "127.0.0.1" {
		t.Fatalf("fetched tunnel mismatch: %+v", fetched)
	}
}

func TestHostDeleteCascadeJumpHosts(t *testing.T) {
	services := setupTestServices(t)
	hostSvc := services.HostService()

	// 1. 创建跳板机 A
	hostA := config.Host{
		Name: "jump-a",
		Host: "jump-a.example.com",
		Port: 22,
		User: "root",
		Auth: config.AuthConfig{Method: "password", Password: "secret"},
	}
	if err := hostSvc.Save(hostA); err != nil {
		t.Fatalf("save host A: %v", err)
	}
	hosts, _ := hostSvc.List()
	idA := hosts[0].ID

	// 2. 创建目标机 B，依赖跳板机 A
	hostB := config.Host{
		Name:        "target-b",
		Host:        "target-b.example.com",
		Port:        22,
		User:        "root",
		Auth:        config.AuthConfig{Method: "password", Password: "secret"},
		JumpHostIDs: []string{idA},
	}
	if err := hostSvc.Save(hostB); err != nil {
		t.Fatalf("save host B: %v", err)
	}

	// 3. 删除跳板机 A
	if err := hostSvc.Delete(idA); err != nil {
		t.Fatalf("delete host A: %v", err)
	}

	// 4. 验证主机 B 的 JumpHostIDs 中 idA 被清理
	hostsAfter, err := hostSvc.List()
	if err != nil || len(hostsAfter) != 1 {
		t.Fatalf("expected 1 host remaining, got %d", len(hostsAfter))
	}
	if len(hostsAfter[0].JumpHostIDs) != 0 {
		t.Fatalf("expected jumpHostIds to be cleaned, got: %v", hostsAfter[0].JumpHostIDs)
	}
}

func TestHostSaveRejectsDanglingAndCyclicJumps(t *testing.T) {
	services := setupTestServices(t)
	hostSvc := services.HostService()
	if err := hostSvc.Save(config.Host{
		Name: "dangling", Host: "dangling.example.com", User: "root",
		JumpHostIDs: []string{"missing"},
	}); err == nil {
		t.Fatal("dangling jump host should be rejected")
	}

	if err := hostSvc.Save(config.Host{Name: "a", Host: "a.example.com", User: "root"}); err != nil {
		t.Fatalf("save host a: %v", err)
	}
	hosts, _ := hostSvc.List()
	if len(hosts) != 1 {
		t.Fatalf("expected one host, got %d", len(hosts))
	}
	if err := hostSvc.Save(config.Host{
		ID: hosts[0].ID, Name: "a", Host: "a.example.com", User: "root",
		JumpHostIDs: []string{hosts[0].ID},
	}); err == nil {
		t.Fatal("self jump should be rejected")
	}
}

func TestValidateCreateAllowsUnixRemoteTargets(t *testing.T) {
	if err := validateCreate(&CreateTunnelRequest{
		Name: "unix-remote", HostID: "host-1", Type: config.TypeRemote,
		RemoteSocket: "/tmp/remote.sock", TargetSocket: "/var/run/service.sock",
	}); err != nil {
		t.Fatalf("unix remote target should validate: %v", err)
	}
}

func TestValidateCreateRejectsConflictingSocketAndPort(t *testing.T) {
	if err := validateCreate(&CreateTunnelRequest{
		Name: "conflict", HostID: "host-1", Type: config.TypeLocal,
		LocalPort: 8080, LocalSocket: "/tmp/local.sock",
		TargetHost: "127.0.0.1", TargetPort: 80,
	}); err == nil {
		t.Fatal("local port/socket conflict should be rejected")
	}
	if err := validateCreate(&CreateTunnelRequest{
		Name: "conflict", HostID: "host-1", Type: config.TypeRemote,
		RemotePort: 9000, TargetSocket: "/tmp/target.sock", TargetHost: "127.0.0.1",
	}); err == nil {
		t.Fatal("remote target port/socket conflict should be rejected")
	}
}

func TestHostSaveRejectsInvalidAuthMethod(t *testing.T) {
	services := setupTestServices(t)
	err := services.HostService().Save(config.Host{
		Name: "server", Host: "server.example.com", User: "root",
		Auth: config.AuthConfig{Method: "unsupported"},
	})
	if err == nil {
		t.Fatal("unsupported auth method should be rejected")
	}
}

func TestHostServiceTestConnectionSuccess(t *testing.T) {
	services := setupTestServices(t)
	host, port := startPasswordSSHServer(t)
	if err := services.HostService().Save(config.Host{
		Name: "test-server",
		Host: host,
		Port: port,
		User: "testuser",
		Auth: config.AuthConfig{Method: config.AuthMethodPassword, Password: "secret"},
	}); err != nil {
		t.Fatalf("save host: %v", err)
	}
	hosts, err := services.HostService().List()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("list host: %v (%d hosts)", err, len(hosts))
	}

	result, err := services.HostService().TestConnection(hosts[0].ID)
	if err != nil {
		t.Fatalf("test connection: %v", err)
	}
	if !result.Success || result.Error != "" {
		t.Fatalf("expected successful probe, got %+v", result)
	}
	if result.DurationMs < 0 {
		t.Fatalf("probe duration must be non-negative: %+v", result)
	}
	if services.Sup.IsRunning("not-a-tunnel") {
		t.Fatal("connectivity probe must not register a tunnel")
	}
}

func TestHostServiceTestConnectionFailureIsResult(t *testing.T) {
	services := setupTestServices(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	_ = listener.Close()
	if err := services.HostService().Save(config.Host{
		Name: "offline",
		Host: host,
		Port: port,
		User: "testuser",
		Auth: config.AuthConfig{Method: config.AuthMethodPassword, Password: "secret"},
	}); err != nil {
		t.Fatalf("save host: %v", err)
	}
	hosts, _ := services.HostService().List()
	result, err := services.HostService().TestConnection(hosts[0].ID)
	if err != nil {
		t.Fatalf("connection failure should be represented in result: %v", err)
	}
	if result.Success || result.Error == "" {
		t.Fatalf("expected failed probe result, got %+v", result)
	}
}

func TestHostServiceTestConnectionRejectsUnknownHost(t *testing.T) {
	services := setupTestServices(t)
	if _, err := services.HostService().TestConnection("missing"); err == nil {
		t.Fatal("unknown host should return an API error")
	}
}
