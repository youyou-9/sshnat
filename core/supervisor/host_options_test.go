package supervisor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sshnat/sshnat/core/config"
)

func TestBuildDialOptionsMapsHostSecurityAndTimeoutPerHop(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	host := config.Host{ID: "target", Host: "target", User: "u", HostKeyPolicy: config.HostKeyStrict, KnownHostsFile: "~/.ssh/trusted_hosts", ConnectTimeoutSeconds: 240, KeepaliveSeconds: -1, JumpHostIDs: []string{"jump"}}
	jump := config.Host{ID: "jump", Host: "jump", User: "u", HostKeyPolicy: config.HostKeyAcceptNew, ConnectTimeoutSeconds: 7}
	if err := store.Save(&config.Settings{Version: 1, Hosts: []config.Host{host, jump}}); err != nil {
		t.Fatal(err)
	}
	opts, err := buildDialOptions(store, host)
	if err != nil {
		t.Fatal(err)
	}
	if opts.HostKeyPolicy != "strict" || opts.KnownHostsFile != "~/.ssh/trusted_hosts" || opts.Timeout != 240*time.Second || opts.Keepalive >= 0 {
		t.Fatalf("target options did not retain host policy: %+v", opts)
	}
	if len(opts.JumpHosts) != 1 || opts.JumpHosts[0].HostKeyPolicy != "accept-new" || opts.JumpHosts[0].Timeout != 7*time.Second {
		t.Fatalf("jump options did not retain independent policy: %+v", opts.JumpHosts)
	}
}
