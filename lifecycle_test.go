package main

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sshnat/sshnat/app"
	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

func TestShutdownStopsHandshakeAndRunsOnlyOnce(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	store := config.NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err := store.Save(&config.Settings{Version: 1, Hosts: []config.Host{{ID: "host", Name: "test", Host: host, Port: port, User: "test"}}, Tunnels: []config.Tunnel{{ID: "tunnel", Name: "test", HostID: "host", Type: "L", LocalPort: 8080, TargetPort: 80}}}); err != nil {
		t.Fatal(err)
	}
	reg := stats.NewRegistry()
	sup := supervisor.New(store, reg)
	services := app.NewServices(store, sup, reg)
	if err := services.TunnelService().Start("tunnel"); err != nil {
		t.Fatal(err)
	}
	statsStops := 0
	var updates nativeUpdateGate
	shutdown := makeShutdownHandler(services, func() { statsStops++ }, updates.Close)
	defer shutdown()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("SSH connection was not established")
	}
	finished := make(chan struct{})
	go func() { shutdown(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown failed to interrupt the SSH handshake")
	}
	shutdown() // Wails hook plus main defer must be safe to call twice.
	if statsStops != 1 || sup.HasActiveRun("tunnel") || !updates.Closed() {
		t.Fatal("shutdown did not stop all runtime work exactly once")
	}
	if got := sup.Status("tunnel"); got != supervisor.StatusStopped {
		t.Fatalf("tunnel state after shutdown = %s", got)
	}
	if err := services.TunnelService().Start("tunnel"); err == nil {
		t.Fatal("queued action restarted a tunnel after shutdown")
	}
}

func TestNativeUpdateGateSerializesShutdownAndDropsQueuedUpdates(t *testing.T) {
	var gate nativeUpdateGate
	started := make(chan struct{})
	finish := make(chan struct{})
	updateDone := make(chan struct{})
	go func() {
		gate.Run(func() {
			close(started)
			<-finish
		})
		close(updateDone)
	}()
	<-started
	closed := make(chan struct{})
	go func() { gate.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("shutdown did not wait for the active native update")
	case <-time.After(20 * time.Millisecond):
	}
	close(finish)
	<-updateDone
	<-closed
	gate.Run(func() { t.Error("queued native update ran after shutdown") })
}
