package ssh

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type blockedUnixClient struct {
	gossh.Conn
	entered chan struct{}
	release chan struct{}
}

type jumpTestClient struct {
	*blockedUnixClient
	connection net.Conn
}

func (c *jumpTestClient) Dial(string, string) (net.Conn, error) { return c.connection, nil }

type noDeadlineConnection struct{ net.Conn }

func (*noDeadlineConnection) SetDeadline(time.Time) error { return nil }

func TestJumpHandshakeUsesPerHopTimeout(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	raw := &jumpTestClient{connection: &noDeadlineConnection{Conn: client}}
	options := &DialOptions{Host: "jump", User: "u", Timeout: 25 * time.Millisecond}
	cfg := &sshClientConfig{User: "u", HostKeyCallback: gossh.InsecureIgnoreHostKey()}
	started := time.Now()
	_, err := dialViaJump(t.Context(), raw, options, cfg)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled hop handshake = %v, want DeadlineExceeded", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("hop ignored its own connection timeout")
	}
}

func (*blockedUnixClient) Dial(string, string) (net.Conn, error) {
	return nil, errors.New("unexpected TCP dial")
}
func (*blockedUnixClient) Listen(string, string) (net.Listener, error) {
	return nil, errors.New("unexpected listen")
}
func (*blockedUnixClient) ListenUnix(string) (net.Listener, error) {
	return nil, errors.New("unexpected listen")
}
func (c *blockedUnixClient) OpenChannel(string, []byte) (gossh.Channel, <-chan *gossh.Request, error) {
	close(c.entered)
	<-c.release
	return nil, nil, errors.New("channel request ended")
}

func TestDialContextCancelsUnixChannelOpen(t *testing.T) {
	raw := &blockedUnixClient{entered: make(chan struct{}), release: make(chan struct{})}
	client := &Client{sshRawClient: raw}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.DialContext(ctx, "unix", "/remote/service.sock")
		done <- err
	}()
	<-raw.entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Unix dial cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Unix channel open ignored context cancellation")
	}
	close(raw.release)
}

func TestDialConnContextClosesLateConnection(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	release := make(chan struct{})
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := dialConnContext(ctx, func() (net.Conn, error) {
			close(entered)
			<-release
			return client, nil
		})
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("dial cancellation = %v", err)
	}
	close(release)
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := server.Read(make([]byte, 1)); err == nil {
		t.Fatal("late connection was not closed")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("late connection remained open after cancellation")
	}
}
