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

func TestParseSSHCommandAdvancedOptions(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-o", "stricthostkeychecking YES", "-oCONNECTTIMEOUT=30",
		"-o", "ServerAliveInterval = 0", "-o", `UserKnownHostsFile="~/.ssh/my hosts"`,
		"-o", `IdentityAgent="C:\\Users\\O'Brien\\my agent"`,
		"-J", "gateway@jump.example", "-D1080", "root@target.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, jumps, _, err := SpecToSettingsWithJumps(spec)
	if err != nil || host.HostKeyPolicy != HostKeyStrict || host.ConnectTimeoutSeconds != 30 || host.KeepaliveSeconds != -1 || host.KnownHostsFile != "~/.ssh/my hosts" || host.Auth.Method != AuthMethodAgent || host.Auth.AgentSocket != `C:\Users\O'Brien\my agent` {
		t.Fatalf("advanced options were lost: host=%+v err=%v", host, err)
	}
	if len(jumps) != 1 || jumps[0].HostKeyPolicy != "" || jumps[0].ConnectTimeoutSeconds != 0 || jumps[0].KeepaliveSeconds != 0 || jumps[0].KnownHostsFile != "" || jumps[0].Auth.AgentSocket != "" {
		t.Fatalf("destination-only options were incorrectly propagated to jumps: %+v", jumps)
	}
}

func TestParseSSHCommandAdvancedOptionsFirstValueWins(t *testing.T) {
	spec, err := ParseSSHCommand([]string{"-o", "ConnectTimeout=30", "-o", "connecttimeout=45", "-o", "StrictHostKeyChecking=accept-new", "-o", "stricthostkeychecking=yes", "root@host"})
	if err != nil || spec.ConnectTimeoutSeconds != 30 || spec.HostKeyPolicy != HostKeyAcceptNew {
		t.Fatalf("OpenSSH first-value semantics changed: spec=%+v err=%v", spec, err)
	}
}

func TestParseSSHCommandRejectsUnsupportedAdvancedOptions(t *testing.T) {
	for _, option := range []string{
		"StrictHostKeyChecking=no", "StrictHostKeyChecking=off", "StrictHostKeyChecking=ask",
		"ConnectTimeout=0", "ConnectTimeout=-1", "ConnectTimeout=301", "ConnectTimeout=bad",
		"ServerAliveInterval=-1", "ServerAliveInterval=86401", "ServerAliveInterval=bad",
		"UserKnownHostsFile=", "UserKnownHostsFile=/tmp/first /tmp/second", `IdentityAgent="unfinished`,
		"IdentityAgent=none", "UserKnownHostsFile=none",
	} {
		t.Run(option, func(t *testing.T) {
			if _, err := ParseSSHCommand([]string{"-o", option, "root@host"}); err == nil {
				t.Fatal("unsupported option was silently accepted")
			}
		})
	}
	if _, err := ParseSSHCommand([]string{"-i", "/tmp/key", "-o", "IdentityAgent=/tmp/agent", "root@host"}); err == nil {
		t.Fatal("mixed key and agent authentication was silently imported")
	}
}

func TestSpecToSettingsWithJumpsPreservesProxyJumpTopology(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-i", "~/.ssh/id_ed25519",
		"-J", "jump-user@jump.example:2200,[2001:db8::2]:2201",
		"root@target.example",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target, jumps, _, err := SpecToSettingsWithJumps(spec)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(jumps) != 2 || len(target.JumpHostIDs) != 2 {
		t.Fatalf("jump topology missing: target=%+v jumps=%+v", target, jumps)
	}
	if jumps[0].User != "jump-user" || jumps[0].Host != "jump.example" || jumps[0].Port != 2200 {
		t.Fatalf("bad first jump: %+v", jumps[0])
	}
	if jumps[1].Host != "2001:db8::2" || jumps[1].Port != 2201 || jumps[1].Auth.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("bad IPv6 jump: %+v", jumps[1])
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

func TestParseSSHCommandAcceptsAttachedValues(t *testing.T) {
	spec, err := ParseSSHCommand([]string{"-p2222", "-L8080:db:80", "-D1080", "alice@host"})
	if err != nil {
		t.Fatalf("parse compact command: %v", err)
	}
	if spec.Port != 2222 || len(spec.Forwards) != 2 || spec.Forwards[0].BindPort != 8080 || spec.Forwards[1].BindPort != 1080 {
		t.Fatalf("compact values were not parsed: %+v", spec)
	}
}

func TestParseSSHCommandUnixSocketForwarding(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-L", "/tmp/local.sock:/run/service.sock",
		"-R", "/tmp/remote.sock:/tmp/local.sock",
		"root@host",
	})
	if err != nil {
		t.Fatalf("parse unix forwarding: %v", err)
	}
	if len(spec.Forwards) != 2 {
		t.Fatalf("expected 2 forwards, got %d", len(spec.Forwards))
	}
	if spec.Forwards[0].Socket != "/tmp/local.sock" || spec.Forwards[0].TargetSocket != "/run/service.sock" {
		t.Fatalf("bad local unix forward: %+v", spec.Forwards[0])
	}
	if spec.Forwards[1].Socket != "/tmp/remote.sock" || spec.Forwards[1].TargetSocket != "/tmp/local.sock" {
		t.Fatalf("bad remote unix forward: %+v", spec.Forwards[1])
	}
	_, tunnels := SpecToSettings(spec)
	if tunnels[0].LocalSocket != "/tmp/local.sock" || tunnels[0].TargetSocket != "/run/service.sock" {
		t.Fatalf("bad local tunnel mapping: %+v", tunnels[0])
	}
	if tunnels[1].RemoteSocket != "/tmp/remote.sock" || tunnels[1].TargetSocket != "/tmp/local.sock" {
		t.Fatalf("bad remote tunnel mapping: %+v", tunnels[1])
	}
}

func TestParseSSHCommandUnixListenerToTCPTarget(t *testing.T) {
	spec, err := ParseSSHCommand([]string{"-L", "/tmp/local.sock:db.internal:3306", "root@host"})
	if err != nil {
		t.Fatalf("parse unix-to-tcp forwarding: %v", err)
	}
	if len(spec.Forwards) != 1 {
		t.Fatalf("expected one forward, got %d", len(spec.Forwards))
	}
	f := spec.Forwards[0]
	if f.Socket != "/tmp/local.sock" || f.TargetHost != "db.internal" || f.TargetPort != 3306 || f.TargetSocket != "" {
		t.Fatalf("unexpected unix-to-tcp forward: %+v", f)
	}
}

func TestParseSSHCommandTCPListenerToUnixTarget(t *testing.T) {
	spec, err := ParseSSHCommand([]string{
		"-L", "8080:/run/service.sock",
		"-R", "127.0.0.1:9000:/tmp/target.sock",
		"root@host",
	})
	if err != nil {
		t.Fatalf("parse tcp-to-unix forwarding: %v", err)
	}
	if len(spec.Forwards) != 2 {
		t.Fatalf("expected two forwards, got %d", len(spec.Forwards))
	}
	local, remote := spec.Forwards[0], spec.Forwards[1]
	if local.BindPort != 8080 || local.TargetSocket != "/run/service.sock" || local.Socket != "" {
		t.Fatalf("unexpected local tcp-to-unix forward: %+v", local)
	}
	if remote.BindHost != "127.0.0.1" || remote.BindPort != 9000 || remote.TargetSocket != "/tmp/target.sock" || remote.Socket != "" {
		t.Fatalf("unexpected remote tcp-to-unix forward: %+v", remote)
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
