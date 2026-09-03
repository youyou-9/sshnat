package supervisor

import (
	"path/filepath"
	"testing"

	"github.com/sshnat/sshnat/core/config"
)

func TestBuildDialOptionsDefaultsAndFlattenedJumps(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	settings := &config.Settings{
		Version: 1,
		Hosts: []config.Host{
			{ID: "target", Host: "target", User: "u", KeepaliveSeconds: 7},
			{ID: "jump-a", Host: "a", User: "u"},
			{ID: "jump-b", Host: "b", User: "u", JumpHostIDs: []string{"jump-c"}},
			{ID: "jump-c", Host: "c", User: "u"},
		},
	}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save: %v", err)
	}
	settings.Hosts[0].JumpHostIDs = []string{"jump-a", "jump-b"}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save target: %v", err)
	}

	opts, err := buildDialOptions(store, settings.Hosts[0])
	if err != nil {
		t.Fatalf("build options: %v", err)
	}
	if opts.KnownHostsFile != filepath.Join(filepath.Dir(store.Path()), "known_hosts") {
		t.Fatalf("default known_hosts = %q", opts.KnownHostsFile)
	}
	if opts.Keepalive.Seconds() != 7 {
		t.Fatalf("keepalive = %s", opts.Keepalive)
	}
	if len(opts.JumpHosts) != 3 || opts.JumpHosts[0].Host != "a" || opts.JumpHosts[1].Host != "c" || opts.JumpHosts[2].Host != "b" {
		t.Fatalf("jump chain not flattened: %+v", opts.JumpHosts)
	}
}
