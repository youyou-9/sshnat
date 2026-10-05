package forward

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	return client.(*net.TCPConn), server.(*net.TCPConn)
}

func TestPipePreservesResponseAfterHalfClose(t *testing.T) {
	client, local := tcpPair(t)
	remote, server := tcpPair(t)
	done := make(chan struct{})
	go func() { pipeConns(t.Context(), local, remote); close(done) }()
	if _, err := client.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	server.SetDeadline(time.Now().Add(time.Second))
	request, err := io.ReadAll(server)
	if err != nil || string(request) != "request" {
		t.Fatalf("read half-closed request = %q, %v", request, err)
	}
	if _, err := server.Write([]byte("complete response after EOF")); err != nil {
		t.Fatal(err)
	}
	server.CloseWrite()
	client.SetDeadline(time.Now().Add(time.Second))
	response, err := io.ReadAll(client)
	if err != nil || string(response) != "complete response after EOF" {
		t.Fatalf("read response = %q, %v", response, err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pipe did not finish after both EOFs")
	}
}

func TestPipeCancellationEndsHalfCloseWait(t *testing.T) {
	client, local := tcpPair(t)
	remote, server := tcpPair(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { pipeConns(ctx, local, remote); close(done) }()
	client.CloseWrite()
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadAll(server); err != nil {
		t.Fatal(err)
	}
	// The target sends no EOF. Cancellation must still end both directions.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pipe still waited for the target after cancellation")
	}
}
