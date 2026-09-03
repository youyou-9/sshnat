package app

import (
	"path/filepath"
	"testing"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

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
