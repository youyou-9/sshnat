package app

import (
	"net"
	"testing"
	"time"

	"github.com/sshnat/sshnat/core/config"
)

func TestHostConnectionUsesConfiguredHandshakeTimeout(t *testing.T) {
	services := setupTestServices(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Do not send an SSH banner. The client must time out the handshake
		// according to the saved option rather than the former 15s cap.
		buffer := make([]byte, 512)
		for {
			if _, err := conn.Read(buffer); err != nil {
				return
			}
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	if err := services.HostService().Save(config.Host{Name: "Timeout", Host: "127.0.0.1", Port: address.Port, User: "tester", Auth: config.AuthConfig{Method: config.AuthMethodPassword, Password: "test"}, ConnectTimeoutSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	hosts, err := services.HostService().List()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, err := services.HostService().TestConnection(hosts[0].ID)
	elapsed := time.Since(started)
	if err != nil || result.Success || result.Error == "" {
		t.Fatalf("expected handshake failure, got %+v, %v", result, err)
	}
	if elapsed < 750*time.Millisecond || elapsed > 4*time.Second {
		t.Fatalf("1s configured timeout was not applied: elapsed=%s", elapsed)
	}
}

func TestHostConnectionStrictPolicyUsesLearnedTrustedRecord(t *testing.T) {
	services := setupTestServices(t)
	address, port := startPasswordSSHServer(t)
	host := config.Host{Name: "Strict", Host: address, Port: port, User: "testuser", Auth: config.AuthConfig{Method: config.AuthMethodPassword, Password: "secret"}, HostKeyPolicy: config.HostKeyStrict}
	if err := services.HostService().Save(host); err != nil {
		t.Fatal(err)
	}
	hosts, _ := services.HostService().List()
	host = hosts[0]
	result, err := services.HostService().TestConnection(host.ID)
	if err != nil || result.Success {
		t.Fatalf("strict first connection should fail without trusted record: %+v, %v", result, err)
	}
	host.HostKeyPolicy = config.HostKeyAcceptNew
	if err := services.HostService().Save(host); err != nil {
		t.Fatal(err)
	}
	result, err = services.HostService().TestConnection(host.ID)
	if err != nil || !result.Success {
		t.Fatalf("accept-new handshake should learn record: %+v, %v", result, err)
	}
	host.HostKeyPolicy = config.HostKeyStrict
	if err := services.HostService().Save(host); err != nil {
		t.Fatal(err)
	}
	result, err = services.HostService().TestConnection(host.ID)
	if err != nil || !result.Success {
		t.Fatalf("strict handshake should accept learned key: %+v, %v", result, err)
	}
}
