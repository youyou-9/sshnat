package forward

import (
	"bytes"
	"io"
	"testing"
)

type countingTraffic struct{ tx, rx uint64 }

func (t *countingTraffic) AddTx(n uint64) { t.tx += n }
func (t *countingTraffic) AddRx(n uint64) { t.rx += n }
func (*countingTraffic) ConnOpened()      {}
func (*countingTraffic) ConnClosed()      {}

type testRWC struct{ bytes.Buffer }

func (*testRWC) Close() error { return nil }

func TestCountingConnDirections(t *testing.T) {
	traffic := &countingTraffic{}
	rwc := &testRWC{}
	inbound := &countingConn{ReadWriteCloser: rwc, t: traffic, readTx: true}
	// Writing to inbound represents Rx
	if _, err := inbound.Write([]byte("rx")); err != nil {
		t.Fatalf("write inbound: %v", err)
	}
	// Reading from inbound represents Tx
	rwc.WriteString("tx")
	buf := make([]byte, 2)
	if _, err := io.ReadFull(inbound, buf); err != nil {
		t.Fatalf("read inbound: %v", err)
	}
	if traffic.tx != 2 || traffic.rx != 2 {
		t.Fatalf("direction counters wrong: tx=%d rx=%d", traffic.tx, traffic.rx)
	}
}
