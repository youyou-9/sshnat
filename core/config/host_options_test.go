package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestHostAdvancedOptionsRoundTrip(t *testing.T) {
	host := Host{ID: "target", Name: "Target", Host: "localhost", User: "user", HostKeyPolicy: HostKeyStrict, ConnectTimeoutSeconds: 300, KeepaliveSeconds: -1, KnownHostsFile: "~/.ssh/known_hosts", JumpHostIDs: []string{"near", "far"}}
	settings := &Settings{Version: 1, Hosts: []Host{
		host,
		{ID: "near", Name: "Near", Host: "near.invalid", User: "user"},
		{ID: "far", Name: "Far", Host: "far.invalid", User: "user"},
	}}
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err := Validate(settings); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got.Hosts[0], host) {
		t.Fatalf("advanced fields did not round-trip: %+v, %v", got, err)
	}
}

func TestValidateHostAdvancedOptions(t *testing.T) {
	for _, options := range []struct {
		timeout int
		policy  string
		valid   bool
	}{
		{0, "", true}, {1, HostKeyAcceptNew, true}, {300, HostKeyStrict, true},
		{-1, HostKeyAcceptNew, false}, {301, HostKeyStrict, false}, {15, "skip", false},
	} {
		host := Host{Name: "Host", Host: "localhost", User: "user", ConnectTimeoutSeconds: options.timeout, HostKeyPolicy: options.policy}
		if err := ValidateHost(host); (err == nil) != options.valid {
			t.Fatalf("ValidateHost(timeout=%d policy=%q) = %v", options.timeout, options.policy, err)
		}
	}
}
