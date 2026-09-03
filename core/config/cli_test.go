package config

import (
	"reflect"
	"testing"
)

func TestParseSSHCommandLocalForward(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-p", "2222", "-i", "~/.ssh/id_ed25519",
		"-L", "8080:db.internal:3306", "-L", "127.0.0.1:9090:web:80",
		"-N", "alice@bastion.example.com",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.User != "alice" || spec.Hostname != "bastion.example.com" || spec.Port != 2222 {
		t.Fatalf("bad target: %+v", spec)
	}
	if spec.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("bad key path: %q", spec.KeyPath)
	}
	want := []ForwardSpec{
		{Mode: "L", BindPort: 8080, TargetHost: "db.internal", TargetPort: 3306},
		{Mode: "L", BindHost: "127.0.0.1", BindPort: 9090, TargetHost: "web", TargetPort: 80},
	}
	if !reflect.DeepEqual(spec.Forwards, want) {
		t.Fatalf("forwards mismatch:\n got %+v\nwant %+v", spec.Forwards, want)
	}
}

func TestParseSSHCommandDynamicAndJumps(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-D", "1080", "-J", "jump1:22,jump2", "bob@target.corp",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(spec.Jumps) != 2 || spec.Jumps[0] != "jump1:22" || spec.Jumps[1] != "jump2" {
		t.Fatalf("bad jumps: %v", spec.Jumps)
	}
	if len(spec.Forwards) != 1 || spec.Forwards[0].Mode != "D" || spec.Forwards[0].BindPort != 1080 {
		t.Fatalf("bad -D forward: %+v", spec.Forwards)
	}
}

func TestParseSSHCommandErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-L", "notaport:x:1", "h"},
		{"-L", "0:x:1", "h"},
		{"-L", "1::1", "h"},
		{"-D", "65536", "h"},
		{"-p", "0", "h"},
		{"-L"}, // 缺值
		{"-N"}, // 无目标
	} {
		if _, err := ParseSSHCommand(args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestParseSSHCommandAcceptsProgramName(t *testing.T) {
	spec, err := ParseSSHCommand([]string{"ssh.exe", "-L", "8080:db:80", "alice@host"})
	if err != nil {
		t.Fatalf("parse command with argv0: %v", err)
	}
	if spec.Hostname != "host" || spec.User != "alice" || len(spec.Forwards) != 1 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestSpecToSettings(t *testing.T) {
	spec, err := ParseSSHCommand([]string{"-L", "5432:db:5432", "root@db.jump"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	host, tunnels := SpecToSettings(spec)
	if host.User != "root" || host.Host != "db.jump" || host.Port != 22 {
		t.Fatalf("bad host: %+v", host)
	}
	if len(tunnels) != 1 {
		t.Fatalf("want 1 tunnel, got %d", len(tunnels))
	}
	tun := tunnels[0]
	if tun.Type != TypeLocal || tun.HostID != host.ID ||
		tun.LocalPort != 5432 || tun.TargetHost != "db" || tun.TargetPort != 5432 {
		t.Fatalf("bad tunnel: %+v", tun)
	}
	if tun.ID == "" || tun.Name == "" {
		t.Fatalf("tunnel must have id/name: %+v", tun)
	}
}

func TestParseSSHCommandIPv6AndTrailingCommand(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-L", "[::1]:8080:[2001:db8::1]:3306",
		"-D", "[::1]:1080",
		"user@bastion.example.com",
		"sleep", "infinity",
	})
	if err != nil {
		t.Fatalf("parse IPv6: %v", err)
	}
	if spec.Hostname != "bastion.example.com" || spec.User != "user" {
		t.Fatalf("target was overwritten by trailing command: %+v", spec)
	}
	if len(spec.Forwards) != 2 {
		t.Fatalf("expected 2 forwards, got %d", len(spec.Forwards))
	}
	if spec.Forwards[0].BindHost != "::1" || spec.Forwards[0].TargetHost != "2001:db8::1" {
		t.Fatalf("IPv6 -L parse failed: %+v", spec.Forwards[0])
	}
	if spec.Forwards[1].BindHost != "::1" || spec.Forwards[1].BindPort != 1080 {
		t.Fatalf("IPv6 -D parse failed: %+v", spec.Forwards[1])
	}
}
