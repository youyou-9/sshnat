// Package forward 实现各类 SSH 端口转发。
//
// Forwarder 是统一的生命周期接口：Start 后在后台监听并转发，
// Stop 幂等关闭监听器与所有活跃连接，Done 在转发器终止后关闭。
package forward

import (
	"errors"
	"io"
	"sync"

	"github.com/sshnat/sshnat/core/stats"
)

// ErrNotImplemented 保留给未来尚未支持的转发扩展。
var ErrNotImplemented = errors.New("forward: not implemented in this build")

// Traffic 是转发器向 stats 上报的最小接口，避免硬依赖具体实现。
var _ Traffic = (*stats.Counter)(nil)

// Traffic 计数接口。
type Traffic interface {
	AddTx(n uint64)
	AddRx(n uint64)
	ConnOpened()
	ConnClosed()
}

// Forwarder 是所有转发器的统一接口。
type Forwarder interface {
	// Start 开始监听。重复调用返回错误。
	Start() error
	// Stop 停止监听并关闭所有连接，幂等。
	Stop() error
	// Done 在转发器完全终止后关闭。
	Done() <-chan struct{}
	// Err 返回终止原因（正常停止返回 nil）。
	Err() error
	// LocalAddr 返回实际监听地址。
	LocalAddr() string
}

// countingConn 包装 net.Conn，按方向计数流量。
type countingConn struct {
	io.ReadWriteCloser
	t      Traffic
	readTx bool // true: Read is Tx, Write is Rx; false: Read is Rx, Write is Tx
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	if n > 0 {
		if c.readTx {
			c.t.AddTx(uint64(n))
		} else {
			c.t.AddRx(uint64(n))
		}
	}
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Write(p)
	if n > 0 {
		if c.readTx {
			c.t.AddRx(uint64(n))
		} else {
			c.t.AddTx(uint64(n))
		}
	}
	return n, err
}

const copyBufferSize = 32 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, copyBufferSize)
		return &buf
	},
}

// pipeConns 双向搬运两个连接的数据，使用对象池复用 32KB 内存缓冲区以降低 GC 开销。
// 任一侧结束即关闭双侧，返回时两侧连接均已关闭。
func pipeConns(a, b io.ReadWriteCloser, t Traffic) {
	done := make(chan struct{}, 2)
	copyHalf := func(dst io.WriteCloser, src io.Reader) {
		defer func() { done <- struct{}{} }()
		bufPtr := copyBufferPool.Get().(*[]byte)
		defer copyBufferPool.Put(bufPtr)
		_, _ = io.CopyBuffer(dst, src, *bufPtr)
	}
	go copyHalf(a, b)
	go copyHalf(b, a)
	<-done
	_ = a.Close()
	_ = b.Close()
	<-done
}
