// Package stats 提供并发安全的每隧道流量/连接计数器。
package stats

import "sync"

// Snapshot 是某一时刻的计数快照。
type Snapshot struct {
	Tx         uint64 `json:"tx"`        // 累计发送字节
	Rx         uint64 `json:"rx"`        // 累计接收字节
	ActiveConn int64  `json:"activeConn"` // 当前活跃连接数
	TotalConn  uint64 `json:"totalConn"`  // 累计连接数
}

// Counter 是单个隧道的计数器，所有方法并发安全。
type Counter struct {
	tx, rx       atomicUint64
	totalConn    atomicUint64
	activeConn   atomicInt64
}

func NewCounter() *Counter { return &Counter{} }

func (c *Counter) AddTx(n uint64) { c.tx.add(n) }
func (c *Counter) AddRx(n uint64) { c.rx.add(n) }

func (c *Counter) ConnOpened() {
	c.totalConn.add(1)
	c.activeConn.add(1)
}

func (c *Counter) ConnClosed() { c.activeConn.sub(1) }

func (c *Counter) Snapshot() Snapshot {
	return Snapshot{
		Tx:         c.tx.load(),
		Rx:         c.rx.load(),
		ActiveConn: c.activeConn.load(),
		TotalConn:  c.totalConn.load(),
	}
}

// Registry 按隧道 ID 管理计数器。
type Registry struct {
	mu sync.Mutex
	m  map[string]*Counter
}

func NewRegistry() *Registry {
	return &Registry{m: make(map[string]*Counter)}
}

// Get 返回指定隧道的计数器，不存在则创建。
func (r *Registry) Get(id string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok {
		c = NewCounter()
		r.m[id] = c
	}
	return c
}

// Reset 清零指定隧道的计数器（用于隧道重建）。
func (r *Registry) Reset(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id] = NewCounter()
}

// Delete 移除指定隧道的计数器。
func (r *Registry) Delete(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}
