package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommandForShell(t *testing.T) {
	tests := []struct {
		name, shell, command string
		want                 []string
	}{
		{
			name: "POSIX copied apostrophe and spaces", shell: "posix",
			command: `ssh -i '~/.ssh/my key' -N -R '/run/o'\''brien.sock:/tmp/service;$x.sock' root@host`,
			want:    []string{"-i", "~/.ssh/my key", "-N", "-R", "/run/o'brien.sock:/tmp/service;$x.sock", "root@host"},
		},
		{
			name: "PowerShell copied apostrophe and Windows key", shell: "powershell",
			command: `ssh -i 'C:\Users\O''Brien\my key' -N -R '/run/o''brien.sock:/tmp/service;$x.sock' root@host`,
			want:    []string{"-i", `C:\Users\O'Brien\my key`, "-N", "-R", "/run/o'brien.sock:/tmp/service;$x.sock", "root@host"},
		},
		{
			name: "PowerShell literal unquoted Windows backslashes", shell: "powershell",
			command: `C:\Windows\System32\OpenSSH\SSH.EXE -i C:\Users\me\key -D1080 root@host`,
			want:    []string{"-i", `C:\Users\me\key`, "-D1080", "root@host"},
		},
		{
			name: "POSIX quoted Windows backslashes", shell: "posix",
			command: `ssh -i 'C:\Users\me\key' -D1080 root@host`,
			want:    []string{"-i", `C:\Users\me\key`, "-D1080", "root@host"},
		},
		{
			name: "POSIX adjacent quotes", shell: "posix",
			command: `ssh -i 'a''b' -D1080 root@host`,
			want:    []string{"-i", "ab", "-D1080", "root@host"},
		},
		{
			name: "PowerShell doubled quotes", shell: "powershell",
			command: `ssh -i 'a''b' -D1080 root@host`,
			want:    []string{"-i", "a'b", "-D1080", "root@host"},
		},
		{
			name: "POSIX escaped whitespace and line continuation", shell: "posix",
			command: "ssh -i my\\ key \\\r\n -D1080 root@host",
			want:    []string{"-i", "my key", "-D1080", "root@host"},
		},
		{
			name: "PowerShell escaped whitespace and line continuation", shell: "powershell",
			command: "ssh -i my` key `\r\n -D1080 root@host",
			want:    []string{"-i", "my key", "-D1080", "root@host"},
		},
		{
			name: "empty arguments reach option validation", shell: "posix",
			command: `ssh -i '' -D1080 root@host`,
			want:    []string{"-i", "", "-D1080", "root@host"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := splitCommandForShell(test.command, test.shell)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSplitCommandForShellErrors(t *testing.T) {
	for _, shell := range []string{"posix", "powershell"} {
		if _, err := splitCommandForShell(`ssh -i 'unfinished`, shell); err != errUnbalancedQuote {
			t.Fatalf("%s unbalanced quote: %v", shell, err)
		}
		if _, err := splitCommandForShell("ssh", shell); err != errEmptyCommand {
			t.Fatalf("%s empty command: %v", shell, err)
		}
	}
	for shell, command := range map[string]string{"posix": "ssh host\\", "powershell": "ssh host`"} {
		if _, err := splitCommandForShell(command, shell); err != errDanglingEscape {
			t.Fatalf("%s dangling escape: %v", shell, err)
		}
	}
	if _, err := splitCommandForShell("ssh host", "unknown"); err == nil || !strings.Contains(err.Error(), "unsupported command shell") {
		t.Fatalf("unknown shell: %v", err)
	}
}

func TestCreateFromSSHCommandForShell(t *testing.T) {
	services := setupTestServices(t)
	tunnelSvc := services.TunnelService()
	for shell, command := range map[string]string{
		"posix":      `ssh -i '~/.ssh/my key' -L '/tmp/o'\''brien.sock:/run/target.sock' root@posix.example`,
		"powershell": `ssh -i '~/.ssh/my key' -L '/tmp/o''brien.sock:/run/target.sock' root@powershell.example`,
	} {
		views, err := tunnelSvc.CreateFromSSHCommandForShell(command, shell)
		if err != nil {
			t.Fatalf("%s import: %v", shell, err)
		}
		if len(views) != 1 || views[0].LocalSocket != "/tmp/o'brien.sock" || views[0].TargetSocket != "/run/target.sock" {
			t.Fatalf("%s import lost socket paths: %#v", shell, views)
		}
		settings, err := services.Store.Load()
		if err != nil {
			t.Fatal(err)
		}
		host, ok := settings.FindHost(views[0].HostID)
		if !ok || host.Auth.KeyPath != "~/.ssh/my key" {
			t.Fatalf("%s import lost key path: %#v", shell, host)
		}
	}
}
