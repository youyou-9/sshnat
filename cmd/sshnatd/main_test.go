package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sshnat/sshnat/core/config"
)

// Run main in a child test process so its real flag parsing and exit code are
// exercised without letting log.Fatal terminate the test runner.
func TestDaemonCLIHelper(t *testing.T) {
	if os.Getenv("SSHNAT_TEST_CLI") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet("sshnatd", flag.ExitOnError)
	main()
	os.Exit(0)
}

func TestDaemonCheckModeDoesNotStartNetworking(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	// The address is deliberately unusable. A successful check proves this
	// mode returns after structural validation and before SSH dialing.
	document := `{"version":1,"hosts":[{"id":"host","name":"Offline","host":"unreachable.invalid","port":22,"user":"tester","auth":{"method":"agent"}}],"tunnels":[{"id":"tunnel","name":"Test","hostId":"host","type":"L","localPort":8080,"targetHost":"127.0.0.1","targetPort":80}]}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runCLI(t, "-config", path, "-check")
	if err != nil || !strings.Contains(output, "config valid: 1 host(s), 1 tunnel(s)") {
		t.Fatalf("check output = %q, err = %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(root, "known_hosts")); !os.IsNotExist(err) {
		t.Fatalf("-check must not write known_hosts: %v", err)
	}
}

func TestDaemonCLIRejectsInvalidConfiguration(t *testing.T) {
	for _, document := range []string{
		`null`,
		`{"version":2}`,
		`{"version":1,"unexpected":true}`,
		`{"version":1,"tunnels":[{"id":"bad","name":"Bad","hostId":"missing","type":"D","socksPort":1080}]}`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := runCLI(t, "-config", path, "-check"); err == nil {
			t.Fatalf("invalid document succeeded: %s", output)
		}
		if output, err := runCLI(t, "-config", path, "-all"); err == nil {
			t.Fatalf("normal daemon startup accepted invalid document: %s", output)
		}
	}
}

func TestDaemonCLIMissingConfigAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	if output, err := runCLI(t, "-config", path, "-check"); err == nil {
		t.Fatalf("missing configuration succeeded: %s", output)
	}
	if output, err := runCLI(t, "-config", path, "-version"); err != nil || !strings.HasPrefix(output, "SSHNat ") {
		t.Fatalf("version mode should not read config: %q, %v", output, err)
	}
}

func TestDaemonImportWithoutExplicitUserCreatesValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	output, err := runCLI(t, "-config", path, "-from-ssh-cmd", "ssh -L 8080:127.0.0.1:80 example.invalid")
	if err != nil {
		t.Fatalf("command import failed: %q, %v", output, err)
	}
	settings, err := readDaemonSettings(path)
	if err != nil || len(settings.Hosts) != 1 || settings.Hosts[0].User == "" || len(settings.Tunnels) != 1 {
		t.Fatalf("command import did not create valid settings: %+v, %v", settings, err)
	}
}

func TestDaemonImportPreservesUnsupportedExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	document := `{"version":1,"futureOption":true}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runCLI(t, "-config", path, "-from-ssh-cmd", "ssh -L 8080:127.0.0.1:80 root@example.invalid"); err == nil {
		t.Fatalf("import accepted unknown config fields: %s", output)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != document {
		t.Fatalf("import changed unsupported config: %s, %v", got, err)
	}
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDaemonCLIHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "SSHNAT_TEST_CLI=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("daemon CLI did not exit: %s", output)
	}
	return string(output), err
}

func TestSelectTunnels(t *testing.T) {
	settings := &config.Settings{Tunnels: []config.Tunnel{
		{ID: "auto", AutoStart: true},
		{ID: "manual", AutoStart: false},
	}}
	for _, tc := range []struct {
		name      string
		all       bool
		requested string
		auto      bool
		want      []string
		wantErr   bool
	}{
		{name: "default auto", auto: true, want: []string{"auto"}},
		{name: "all", all: true, want: []string{"auto", "manual"}},
		{name: "explicit deduplicated", requested: " manual,auto,manual ", want: []string{"manual", "auto"}},
		{name: "missing ID", requested: "auto,missing", wantErr: true},
		{name: "empty ID", requested: "auto,", wantErr: true},
		{name: "conflicting selection", all: true, requested: "manual", wantErr: true},
		{name: "no selection", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectTunnels(settings, tc.all, tc.requested, tc.auto)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tc.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selection = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDescribeTunnelSupportsIPv6AndMixedSocketRoutes(t *testing.T) {
	for _, tc := range []struct {
		tunnel config.Tunnel
		want   string
	}{
		{config.Tunnel{Type: config.TypeLocal, LocalSocket: "/tmp/local.sock", TargetHost: "::1", TargetPort: 80}, "-L /tmp/local.sock -> [::1]:80"},
		{config.Tunnel{Type: config.TypeRemote, RemoteBindHost: "::", RemotePort: 9000, TargetSocket: "/tmp/local.sock"}, "-R [::]:9000 <- /tmp/local.sock"},
		{config.Tunnel{Type: config.TypeRemote, RemoteSocket: "/tmp/remote.sock", TargetHost: "localhost", TargetPort: 3000}, "-R /tmp/remote.sock <- localhost:3000"},
		{config.Tunnel{Type: config.TypeDynamic, LocalBindHost: "::1", SocksPort: 1080}, "-D socks5://[::1]:1080"},
	} {
		if got := describeTunnel(tc.tunnel); got != tc.want {
			t.Fatalf("describeTunnel(%+v) = %q, want %q", tc.tunnel, got, tc.want)
		}
	}
}
