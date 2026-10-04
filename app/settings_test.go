package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sshnat/sshnat/core/config"
)

func importFixture() *config.Settings {
	return &config.Settings{Version: 1, Hosts: []config.Host{
		{ID: "jump", Name: "Jump", Host: "jump.example", User: "alice", Auth: config.AuthConfig{Method: "password", Password: "backup-secret"}},
		{ID: "dest", Name: "Destination", Host: "dest.example", User: "alice", JumpHostIDs: []string{"jump"}, Auth: config.AuthConfig{Method: "key", KeyPath: "~/.ssh/id_ed25519", KeyPassphrase: "backup-passphrase"}},
	}, Tunnels: []config.Tunnel{{ID: "t", Name: "service", HostID: "dest", Type: "L", LocalPort: 8080, TargetSocket: "/run/service.sock"}}}
}

func TestSettingsExportRedactsWithoutChangingStore(t *testing.T) {
	s := setupTestServices(t)
	if err := s.Store.Save(importFixture()); err != nil {
		t.Fatal(err)
	}
	redacted, err := s.SettingsService().Export(false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(redacted, "backup-secret") || strings.Contains(redacted, "backup-passphrase") {
		t.Fatal("export leaked credentials")
	}
	if _, err := config.ParseSettings([]byte(redacted)); err != nil {
		t.Fatalf("export cannot be imported: %v", err)
	}
	full, err := s.SettingsService().Export(true)
	if err != nil || !strings.Contains(full, "backup-secret") || !strings.Contains(full, "backup-passphrase") {
		t.Fatal("full backup lost credentials")
	}
}

func TestSettingsExportRejectsActiveFileAlias(t *testing.T) {
	s := setupTestServices(t)
	if err := s.Store.Save(importFixture()); err != nil {
		t.Fatal(err)
	}
	alias := s.Store.Path() + ".alias"
	if err := os.Link(s.Store.Path(), alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	s.SetExportChooser(func() (string, error) { return alias, nil })
	if _, err := s.SettingsService().ExportFile(false); err == nil {
		t.Fatal("active file alias was accepted as a backup destination")
	}
	document, err := s.SettingsService().Export(true)
	if err != nil || !strings.Contains(document, "backup-secret") {
		t.Fatal("export through alias changed active credentials")
	}
}

func TestSettingsMergeRemapsCollidingIDsAndJumpReferences(t *testing.T) {
	s := setupTestServices(t)
	fixture := importFixture()
	if err := s.Store.Save(fixture); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(fixture)
	result, err := s.SettingsService().Import(string(data), "merge")
	if err != nil {
		t.Fatal(err)
	}
	if result.HostsAdded != 2 || result.TunnelsAdded != 1 {
		t.Fatalf("wrong import counts: %+v", result)
	}
	settings, _ := s.Store.Load()
	if len(settings.Hosts) != 4 || len(settings.Tunnels) != 2 {
		t.Fatal("merge lost entries")
	}
	if err := config.Validate(settings); err != nil {
		t.Fatal(err)
	}
	if settings.Hosts[3].JumpHostIDs[0] != settings.Hosts[2].ID || settings.Tunnels[1].HostID != settings.Hosts[3].ID {
		t.Fatal("import topology was not remapped")
	}
	if settings.Hosts[0].Auth.Password != fixture.Hosts[0].Auth.Password {
		t.Fatal("merge changed existing credentials")
	}
}

func TestSettingsReplaceBacksUpAndInvalidImportIsAtomic(t *testing.T) {
	s := setupTestServices(t)
	if err := s.Store.Save(importFixture()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Store.Path())
	if _, err := s.SettingsService().Import(`{"version":2}`, "replace"); err == nil {
		t.Fatal("invalid schema accepted")
	}
	after, _ := os.ReadFile(s.Store.Path())
	if string(before) != string(after) {
		t.Fatal("invalid import changed configuration")
	}
	result, err := s.SettingsService().Import(`{"version":1,"hosts":[],"tunnels":[]}`, "replace")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil || string(backup) != string(before) {
		t.Fatal("backup does not preserve exact previous configuration")
	}
	settings, _ := s.Store.Load()
	if len(settings.Hosts) != 0 || len(settings.Tunnels) != 0 {
		t.Fatal("replace did not apply")
	}
}

func TestSettingsExportFileUsesPrivateBackupAndRejectsActiveConfig(t *testing.T) {
	s := setupTestServices(t)
	if err := s.Store.Save(importFixture()); err != nil {
		t.Fatal(err)
	}
	s.SetExportChooser(func() (string, error) { return s.Store.Path(), nil })
	if _, err := s.SettingsService().ExportFile(false); err == nil {
		t.Fatal("export can overwrite active config")
	}
	path := s.Store.Path() + ".export.json"
	s.SetExportChooser(func() (string, error) { return path, nil })
	if _, err := s.SettingsService().ExportFile(false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "backup-secret") {
		t.Fatal("file export leaked credentials")
	}
	s.SetExportChooser(func() (string, error) { return "", nil })
	if path, err := s.SettingsService().ExportFile(true); err != nil || path != "" {
		t.Fatal("cancelled dialog must not save")
	}
}

func TestSocketTunnelRoundTripsExportImport(t *testing.T) {
	s := setupTestServices(t)
	if err := s.HostService().Save(config.Host{Name: "host", Host: "localhost", User: "alice"}); err != nil {
		t.Fatal(err)
	}
	hosts, _ := s.HostService().List()
	created, err := s.TunnelService().Create(CreateTunnelRequest{Name: "unix", HostID: hosts[0].ID, Type: "L", LocalPort: 8080, TargetSocket: "/run/service.sock"})
	if err != nil {
		t.Fatal(err)
	}
	if created.TargetHost != "" {
		t.Fatal("socket target got unwanted TCP host default")
	}
	data, err := s.SettingsService().Export(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SettingsService().Import(data, "merge"); err != nil {
		t.Fatal(err)
	}
}
