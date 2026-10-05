package forward_test

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sshnat/sshnat/core/forward"
	"github.com/sshnat/sshnat/core/ssh"
	"github.com/sshnat/sshnat/core/stats"
)

func localTestClient(t *testing.T) (*ssh.Client, string) {
	t.Helper()
	target := startEchoServer(t)
	srv := startMockSSHServer(t, hostOf(target), "secret")
	client, err := ssh.Dial(t.Context(), ssh.DialOptions{
		Host: hostOf(srv.addr()), Port: portOf(srv.addr()), User: "testuser",
		Auth:           ssh.AuthConfig{Type: ssh.AuthPassword, Password: "secret"},
		KnownHostsFile: filepath.Join(t.TempDir(), "known_hosts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client, target
}

func TestLocalStopBeforeStartPreservesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "important.txt")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	fw := forward.NewLocal(nil, forward.LocalConfig{ListenNetwork: "unix", ListenAddr: path}, stats.NewRegistry().Get("path"))
	if err := fw.Stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("Stop removed an unowned path: data=%q err=%v", data, err)
	}
}

func TestLocalStartPreservesActiveUnixSocket(t *testing.T) {
	client, target := localTestClient(t)
	path := filepath.Join(t.TempDir(), "active.sock")
	existing, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	defer existing.Close()
	fw := forward.NewLocal(client, forward.LocalConfig{ListenNetwork: "unix", ListenAddr: path, TargetAddr: target}, stats.NewRegistry().Get("active"))
	if err := fw.Start(); err == nil {
		fw.Stop()
		t.Fatal("Start replaced an active socket")
	}
	if err := fw.Stop(); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("existing socket was made unreachable: %v", err)
	}
	connection.Close()
}

func TestLocalStopPreservesReplacedSocketPath(t *testing.T) {
	client, target := localTestClient(t)
	path := filepath.Join(t.TempDir(), "owned.sock")
	fw := forward.NewLocal(client, forward.LocalConfig{ListenNetwork: "unix", ListenAddr: path, TargetAddr: target}, stats.NewRegistry().Get("owned"))
	if err := fw.Start(); err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	defer fw.Stop()
	if err := os.Remove(path); err != nil {
		t.Skipf("OS cannot replace open Unix socket path: %v", err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fw.Stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("Stop removed replacement file: data=%q err=%v", data, err)
	}
}

func TestLocalStopRemovesOnlyItsOwnedSocket(t *testing.T) {
	client, target := localTestClient(t)
	path := filepath.Join(t.TempDir(), "owned.sock")
	fw := forward.NewLocal(client, forward.LocalConfig{ListenNetwork: "unix", ListenAddr: path, TargetAddr: target}, stats.NewRegistry().Get("cleanup"))
	if err := fw.Start(); err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	if err := fw.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned socket was not removed: %v", err)
	}
}

func TestLocalStopWaitsForActiveConnectionWorkers(t *testing.T) {
	client, target := localTestClient(t)
	counter := stats.NewRegistry().Get("worker")
	fw := forward.NewLocal(client, forward.LocalConfig{ListenAddr: "127.0.0.1:0", TargetAddr: target}, counter)
	if err := fw.Start(); err != nil {
		t.Fatal(err)
	}
	defer fw.Stop()
	conn, err := net.Dial("tcp", fw.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if counter.Snapshot().ActiveConn != 1 {
		t.Fatal("expected an active connection before Stop")
	}
	if err := fw.Stop(); err != nil {
		t.Fatal(err)
	}
	if counter.Snapshot().ActiveConn != 0 {
		t.Fatal("Stop returned before active connection workers finished")
	}
}
