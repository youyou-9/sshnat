package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/supervisor"
)

func TestTrayToggleUsesCurrentLifecycleAndReturnsErrors(t *testing.T) {
	active := false
	starts, stops := 0, 0
	stopError := errors.New("stop failed")
	click := func() error {
		return toggleTrayTunnel("tunnel", func(id string) bool {
			if id != "tunnel" {
				t.Fatalf("wrong tunnel ID: %s", id)
			}
			return active
		}, func(string) error { starts++; return nil }, func(string) error { stops++; return stopError })
	}
	// The menu was built while stopped, but the run is reconnecting by click
	// time. It must be stopped instead of attempting a duplicate Start.
	active = true
	if err := click(); !errors.Is(err, stopError) {
		t.Fatalf("stop error not propagated: %v", err)
	}
	if starts != 0 || stops != 1 {
		t.Fatal("active run was restarted")
	}
	active = false
	if err := click(); err != nil || starts != 1 || stops != 1 {
		t.Fatal("stopped tunnel did not start")
	}
}

func TestTrayLabelsShowReconnectAndUnixSocketEndpoints(t *testing.T) {
	for _, test := range []struct {
		name   string
		tunnel config.Tunnel
		status supervisor.Status
		want   string
	}{
		{"reconnecting", config.Tunnel{Name: "api", Type: "L", LocalPort: 8080}, supervisor.StatusReconnecting, "[重连中] api (-L: 8080)"},
		{"stopping", config.Tunnel{Name: "api", Type: "L", LocalPort: 8080}, supervisor.StatusStopping, "[停止中] api (-L: 8080)"},
		{"local socket", config.Tunnel{Name: "api", Type: "L", LocalSocket: "/tmp/local.sock"}, supervisor.StatusConnected, "(-L: /tmp/local.sock)"},
		{"remote socket", config.Tunnel{Name: "api", Type: "R", RemoteSocket: "/tmp/remote.sock"}, supervisor.StatusConnected, "(-R: /tmp/remote.sock)"},
		{"socks", config.Tunnel{Name: "proxy", Type: "D", SocksPort: 1080}, supervisor.StatusStopped, "(-D: 1080)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := trayTunnelLabel(test.tunnel, test.status); !strings.Contains(got, test.want) {
				t.Fatalf("tray label = %q, want %q", got, test.want)
			}
		})
	}
}
