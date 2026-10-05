package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sshnat/sshnat/core/config"
)

func TestConfigChangeEventsFollowPersistedCRUDAndImports(t *testing.T) {
	s := setupTestServices(t)
	changes := 0
	s.SetEmitter(func(name string, payload ...any) {
		if name != EventConfigChanged {
			return
		}
		changes++
		if len(payload) != 0 {
			t.Error("configuration notification must not contain credentials")
		}
		// Export takes the same configuration lock as mutations. A consumer
		// must be able to reload here without deadlocking or seeing old data.
		if _, err := s.SettingsService().Export(false); err != nil {
			t.Errorf("reload configuration from notification: %v", err)
		}
	})
	check := func(want int) {
		t.Helper()
		if changes != want {
			t.Fatalf("configuration events = %d, want %d", changes, want)
		}
	}
	host := config.Host{Name: "host", Host: "server.example", User: "alice"}
	if err := s.HostService().Save(host); err != nil {
		t.Fatal(err)
	}
	check(1)
	hosts, _ := s.HostService().List()
	host = hosts[0]
	host.Name = "renamed host"
	if err := s.HostService().Save(host); err != nil {
		t.Fatal(err)
	}
	check(2)
	tunnel, err := s.TunnelService().Create(CreateTunnelRequest{Name: "stopped tunnel", HostID: host.ID, Type: "L", LocalPort: 8080, TargetPort: 80})
	if err != nil {
		t.Fatal(err)
	}
	check(3)
	if _, err := s.TunnelService().Update(UpdateTunnelRequest{ID: tunnel.ID, Name: "renamed stopped tunnel", HostID: host.ID, Type: "L", LocalPort: 8080, TargetPort: 80}); err != nil {
		t.Fatal(err)
	}
	check(4)
	if err := s.TunnelService().Delete(tunnel.ID); err != nil {
		t.Fatal(err)
	}
	check(5)
	if _, err := s.TunnelService().CreateFromSSHCommandForShell("ssh -L8081:localhost:80 alice@server.example", "posix"); err != nil {
		t.Fatal(err)
	}
	check(6)
	if err := s.HostService().Delete(host.ID); err != nil {
		t.Fatal(err)
	}
	check(7)
	settings, _ := s.Store.Load()
	if len(settings.Hosts) != 0 || len(settings.Tunnels) != 0 {
		t.Fatal("host deletion did not remove its stopped tunnel")
	}
	document := `{"version":1,"hosts":[{"id":"imported-host","name":"host","host":"server.example","user":"alice","auth":{}}],"tunnels":[]}`
	if _, err := s.SettingsService().Import(document, "merge"); err != nil {
		t.Fatal(err)
	}
	check(8)
	if _, err := s.SettingsService().Import(`{"version":1,"hosts":[],"tunnels":[]}`, "replace"); err != nil {
		t.Fatal(err)
	}
	check(9)
	if _, err := s.SettingsService().Import("invalid JSON", "replace"); err == nil {
		t.Fatal("invalid import succeeded")
	}
	if err := s.HostService().Save(config.Host{}); err == nil {
		t.Fatal("invalid host succeeded")
	}
	if err := s.TunnelService().Delete("missing"); err == nil {
		t.Fatal("missing tunnel deletion succeeded")
	}
	if _, err := s.SettingsService().Export(false); err != nil {
		t.Fatal(err)
	}
	check(9)
}

func TestFailedConfigurationSaveDoesNotNotify(t *testing.T) {
	s := setupTestServices(t)
	parentFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parentFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s.Store = config.NewStore(filepath.Join(parentFile, "config.json"))
	changes := 0
	s.SetEmitter(func(name string, _ ...any) {
		if name == EventConfigChanged {
			changes++
		}
	})
	if err := s.HostService().Save(config.Host{Name: "host", Host: "server.example", User: "alice"}); err == nil {
		t.Fatal("save below a regular file succeeded")
	}
	if changes != 0 {
		t.Fatalf("failed save emitted %d configuration changes", changes)
	}
}
