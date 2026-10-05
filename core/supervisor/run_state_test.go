package supervisor

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
)

func TestStartCannotReplaceRunAfterStopTimeout(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	settings := &config.Settings{
		Version: 1,
		Hosts:   []config.Host{{ID: "host", Name: "host", Host: "127.0.0.1", Port: 1, User: "u"}},
		Tunnels: []config.Tunnel{{ID: "tunnel", Name: "tunnel", HostID: "host", Type: config.TypeLocal, LocalPort: 59999, TargetPort: 80}},
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	sup := New(store, stats.NewRegistry())
	sup.stopTimeout = 20 * time.Millisecond
	old := &managed{tun: settings.Tunnels[0], st: StatusConnected, stopCh: make(chan struct{}), doneCh: make(chan struct{})}
	sup.tunnels["tunnel"] = old

	if err := sup.Stop("tunnel"); err == nil {
		t.Fatal("a live run must report its stop timeout")
	}
	if sup.IsRunning("tunnel") {
		t.Fatal("the displayed error status should not be shown as running")
	}
	if !sup.HasActiveRun("tunnel") {
		t.Fatal("timed-out run must remain active until doneCh closes")
	}
	if err := sup.Start("tunnel"); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Start while old loop is alive = %v, want ErrAlreadyRunning", err)
	}
	if sup.tunnels["tunnel"] != old {
		t.Fatal("Start replaced a live loop")
	}

	close(old.doneCh)
	sup.stopTimeout = time.Second
	if sup.HasActiveRun("tunnel") {
		t.Fatal("closed doneCh must be reported as inactive")
	}
	if err := sup.Start("tunnel"); err != nil {
		t.Fatalf("Start after old loop exits: %v", err)
	}
	if sup.tunnels["tunnel"] == old {
		t.Fatal("completed loop was not replaced")
	}
	if err := sup.Stop("tunnel"); err != nil {
		t.Fatalf("Stop new loop: %v", err)
	}
}

func TestStopInterruptsRemoteListenerStartup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &gossh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)
	requestSeen := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := gossh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		defer server.Close()
		go func() {
			for channel := range channels {
				channel.Reject(gossh.Prohibited, "no channels")
			}
		}()
		for request := range requests {
			if request.Type == "tcpip-forward" {
				close(requestSeen)
				// Simulate an SSH server that never answers listener creation.
			}
		}
	}()
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	settings := &config.Settings{
		Version: 1,
		Hosts:   []config.Host{{ID: "host", Name: "host", Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, User: "u", KeepaliveSeconds: -1}},
		Tunnels: []config.Tunnel{{ID: "remote", Name: "remote", HostID: "host", Type: config.TypeRemote, RemotePort: 59999, TargetPort: 80}},
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	sup := New(store, stats.NewRegistry())
	sup.stopTimeout = time.Second
	if err := sup.Start("remote"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestSeen:
	case <-time.After(time.Second):
		sup.Stop("remote")
		t.Fatal("SSH server never received remote listen request")
	}
	if err := sup.Stop("remote"); err != nil {
		t.Fatalf("Stop during unanswered remote listen: %v", err)
	}
	if sup.HasActiveRun("remote") {
		t.Fatal("remote run remained alive after Stop")
	}
}
