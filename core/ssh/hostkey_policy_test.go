package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testPublicKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestAcceptNewRejectsChangedKeyUsingOriginalCallbacks(t *testing.T) {
	file := filepath.Join(t.TempDir(), "known_hosts")
	first, err := hostKeyCallback(&DialOptions{KnownHostsFile: file, HostKeyPolicy: "accept-new"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate two connection attempts that created their callbacks before
	// either server key was recorded. A cached empty snapshot was unsafe.
	second, err := hostKeyCallback(&DialOptions{KnownHostsFile: file})
	if err != nil {
		t.Fatal(err)
	}
	address := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	trusted, changed := testPublicKey(t), testPublicKey(t)
	if err := first("127.0.0.1:2222", address, trusted); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := first("127.0.0.1:2222", address, changed); err == nil {
		t.Fatal("accept-new accepted a changed key through the same callback")
	}
	if err := second("127.0.0.1:2222", address, changed); err == nil {
		t.Fatal("a concurrently created callback accepted a changed key")
	}
	if err := second("127.0.0.1:2222", address, trusted); err != nil {
		t.Fatalf("trusted key should still succeed: %v", err)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("changed-key rejection modified trusted records")
	}
}

func TestStrictHostKeyPolicyRequiresKnownMatchingKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "known_hosts")
	trusted := testPublicKey(t)
	contents := knownhosts.Line([]string{"[127.0.0.1]:2222"}, trusted) + "\n"
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	callback, err := hostKeyCallback(&DialOptions{KnownHostsFile: file, HostKeyPolicy: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	address := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	if err := callback("127.0.0.1:2222", address, trusted); err != nil {
		t.Fatalf("strict known key: %v", err)
	}
	if err := callback("127.0.0.1:2222", address, testPublicKey(t)); err == nil {
		t.Fatal("strict accepted a changed key")
	}
	unknown := &net.TCPAddr{IP: net.ParseIP("127.0.0.2"), Port: 2222}
	if err := callback("127.0.0.2:2222", unknown, trusted); err == nil {
		t.Fatal("strict accepted an unknown host")
	}
	after, _ := os.ReadFile(file)
	if string(after) != contents {
		t.Fatal("strict validation must never write to known_hosts")
	}
}

func TestStrictPolicyDoesNotCreateMissingKnownHosts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing", "known_hosts")
	if _, err := hostKeyCallback(&DialOptions{KnownHostsFile: file, HostKeyPolicy: "strict"}); err == nil {
		t.Fatal("strict accepted a missing known_hosts file")
	}
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatalf("strict mode created a directory: %v", err)
	}
}

func TestHostKeyPolicyRejectsUnsupportedMode(t *testing.T) {
	if _, err := hostKeyCallback(&DialOptions{HostKeyPolicy: "skip"}); err == nil {
		t.Fatal("unsupported host key policy accepted")
	}
}
