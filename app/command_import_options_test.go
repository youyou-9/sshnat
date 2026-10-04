package app

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/sshnat/sshnat/core/config"
)

// These commands are also asserted against the actual TypeScript formatter.
// Importing the shared fixtures checks both quoting layers and persistence.
func TestCommandImportAdvancedFormatterRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../frontend/src/lib/test-fixtures/advanced-ssh-commands.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Shell   string
		Command string
		Host    config.Host
		Tunnel  config.Tunnel
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Shell+"/"+fixture.Host.Host, func(t *testing.T) {
			services := setupTestServices(t)
			views, err := services.TunnelService().CreateFromSSHCommandForShell(fixture.Command, fixture.Shell)
			if err != nil || len(views) != 1 {
				t.Fatalf("import: views=%+v err=%v", views, err)
			}
			settings, err := services.Store.Load()
			if err != nil {
				t.Fatal(err)
			}
			host, ok := settings.FindHost(views[0].HostID)
			if !ok {
				t.Fatal("imported host missing")
			}
			wantTimeout, wantKeepalive := fixture.Host.ConnectTimeoutSeconds, fixture.Host.KeepaliveSeconds
			if wantTimeout == 0 {
				wantTimeout = 15
			}
			if wantKeepalive == 0 {
				wantKeepalive = 15
			}
			wantPolicy := fixture.Host.HostKeyPolicy
			if wantPolicy == "" {
				wantPolicy = config.HostKeyAcceptNew
			}
			if host.HostKeyPolicy != wantPolicy || host.ConnectTimeoutSeconds != wantTimeout || host.KeepaliveSeconds != wantKeepalive || host.KnownHostsFile != fixture.Host.KnownHostsFile || host.Auth != fixture.Host.Auth {
				t.Fatalf("advanced options changed: got=%+v want=%+v", host, fixture.Host)
			}
			if views[0].Type != fixture.Tunnel.Type || views[0].SocksPort != fixture.Tunnel.SocksPort {
				t.Fatalf("forward changed: %+v", views[0])
			}
		})
	}
}

func TestCommandImportExplicitOptionsUpdateExistingHost(t *testing.T) {
	services := setupTestServices(t)
	svc := services.TunnelService()
	initial := `ssh -o StrictHostKeyChecking=yes -o ConnectTimeout=30 -o ServerAliveInterval=0 -o UserKnownHostsFile=~/.ssh/trusted -o IdentityAgent=/tmp/agent -D1080 root@target.example`
	first, err := svc.CreateFromSSHCommandForShell(initial, "posix")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateFromSSHCommandForShell(`ssh -o ConnectTimeout=45 -D1081 root@target.example`, "posix")
	if err != nil || len(second) != 1 || second[0].HostID != first[0].HostID {
		t.Fatalf("reuse: views=%+v err=%v", second, err)
	}
	settings, _ := services.Store.Load()
	host, _ := settings.FindHost(first[0].HostID)
	if len(settings.Hosts) != 1 || host.ConnectTimeoutSeconds != 45 || host.HostKeyPolicy != config.HostKeyStrict || host.KeepaliveSeconds != -1 || host.KnownHostsFile != "~/.ssh/trusted" || host.Auth.Method != config.AuthMethodAgent || host.Auth.AgentSocket != "/tmp/agent" {
		t.Fatalf("explicit option failed to update or omitted options were lost: %+v", host)
	}
	if _, err := svc.CreateFromSSHCommandForShell(`ssh -o IdentityAgent=SSH_AUTH_SOCK -D1082 root@target.example`, "posix"); err != nil {
		t.Fatal(err)
	}
	settings, _ = services.Store.Load()
	host, _ = settings.FindHost(first[0].HostID)
	if host.Auth.Method != config.AuthMethodAgent || host.Auth.AgentSocket != "" || host.ConnectTimeoutSeconds != 45 {
		t.Fatalf("default agent option did not reset the explicit socket: %+v", host)
	}
}

func TestCommandImportRejectsUnsafeHostKeyOptionWithoutSaving(t *testing.T) {
	services := setupTestServices(t)
	if _, err := services.TunnelService().CreateFromSSHCommandForShell(`ssh -o StrictHostKeyChecking=no -D1080 root@target.example`, "posix"); err == nil {
		t.Fatal("unsafe host key option was silently imported")
	}
	settings, err := services.Store.Load()
	if err != nil || len(settings.Hosts) != 0 || len(settings.Tunnels) != 0 {
		t.Fatalf("rejected command mutated configuration: settings=%+v err=%v", settings, err)
	}
}

func TestCommandImportExplicitKeyUpdatesExistingHost(t *testing.T) {
	services := setupTestServices(t)
	existing := config.Host{ID: "existing", Name: "Existing", Host: "target.example", Port: 22, User: "root", Auth: config.AuthConfig{Method: config.AuthMethodPassword, Password: "old-password"}}
	if err := services.Store.Save(&config.Settings{Version: 1, Hosts: []config.Host{existing}}); err != nil {
		t.Fatal(err)
	}
	svc := services.TunnelService()
	if _, err := svc.CreateFromSSHCommandForShell(`ssh -i ~/.ssh/new-key -D1080 root@target.example`, "posix"); err != nil {
		t.Fatal(err)
	}
	settings, _ := services.Store.Load()
	host, _ := settings.FindHost(existing.ID)
	if len(settings.Hosts) != 1 || host.Auth.Method != config.AuthMethodKey || host.Auth.KeyPath != "~/.ssh/new-key" || host.Auth.Password != "" || host.Auth.KeyPassphrase != "" {
		t.Fatalf("explicit key did not replace password auth: %+v", host)
	}
	host.Auth.KeyPassphrase = "test-passphrase"
	if err := services.HostService().Save(*host); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFromSSHCommandForShell(`ssh -i ~/.ssh/new-key -D1081 root@target.example`, "posix"); err != nil {
		t.Fatal(err)
	}
	settings, _ = services.Store.Load()
	host, _ = settings.FindHost(existing.ID)
	if host.Auth.KeyPassphrase != "test-passphrase" {
		t.Fatal("reusing the same key lost its saved passphrase")
	}
	if _, err := svc.CreateFromSSHCommandForShell(`ssh -i ~/.ssh/other-key -D1082 root@target.example`, "posix"); err != nil {
		t.Fatal(err)
	}
	settings, _ = services.Store.Load()
	host, _ = settings.FindHost(existing.ID)
	if host.Auth.KeyPassphrase != "" || host.Auth.KeyPath != "~/.ssh/other-key" {
		t.Fatal("a different key kept an inapplicable passphrase")
	}
}
