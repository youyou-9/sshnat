package forward

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestSocks5HandshakeAndDomainRequest(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		if err := socks5Handshake(server); err != nil {
			done <- err
			return
		}
		addr, err := readSocks5Request(server)
		if err == nil && addr != "db.internal:3306" {
			err = &testError{addr}
		}
		done <- err
	}()

	if _, err := client.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 2)
	if _, err := client.Read(response); err != nil {
		t.Fatal(err)
	}
	if response[0] != 5 || response[1] != 0 {
		t.Fatalf("unexpected greeting response: %v", response)
	}
	request := []byte{5, 1, 0, 3, byte(len("db.internal"))}
	request = append(request, []byte("db.internal")...)
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, 3306)
	request = append(request, port...)
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type testError struct{ value string }

func (e *testError) Error() string { return e.value }
