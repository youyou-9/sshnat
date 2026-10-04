package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func TestTransportEOFWakesDoneWithoutKeepalive(t *testing.T) {
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
	cfg := &gossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	accepted := make(chan *gossh.ServerConn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		server, _, _, err := gossh.NewServerConn(conn, cfg)
		if err != nil {
			conn.Close()
			return
		}
		accepted <- server
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	client, err := Dial(t.Context(), DialOptions{
		Host: host, Port: port, User: "u", Auth: AuthConfig{Type: AuthPassword},
		Keepalive: -1, InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	server.Close()
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("transport EOF did not close Client.Done with keepalive disabled")
	}
}
