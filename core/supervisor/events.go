package supervisor

import (
	"sync"
	"time"
)

// Status 隧道运行状态。
type Status string

const (
	StatusStopped      Status = "stopped"      // 未启动
	StatusStarting     Status = "starting"     // 正在建立 SSH 连接
	StatusConnected    Status = "connected"    // 已连接，转发中
	StatusReconnecting Status = "reconnecting" // 断线退避重连中
	StatusStopping     Status = "stopping"     // 正在停止
	StatusError        Status = "error"        // 停止前发生致命错误（用户可再启动）
)

// EventType 事件种类。
type EventType string

const (
	EventStatus EventType = "status"
	EventStats  EventType = "stats"
	EventLog    EventType = "log"
)

// Event 是事件总线上的消息。
type Event struct {
	Type     EventType `json:"type"`
	TunnelID string    `json:"tunnelId"`

	// status 事件字段。
	Status   Status `json:"status,omitempty"`
	Previous Status `json:"previous,omitempty"`
	Error    string `json:"error,omitempty"`
	Attempt  int    `json:"attempt,omitempty"` // 第几次重连尝试
	NextInMs int64  `json:"nextInMs,omitempty"`// 距下次重连毫秒数

	// stats 事件字段：自上次事件以来的增量 + 累计值。
	Tx       uint64 `json:"tx,omitempty"`       // 本周期发送增量
	Rx       uint64 `json:"rx,omitempty"`       // 本周期接收增量
	TxTotal  uint64 `json:"txTotal,omitempty"`  // 累计发送
	RxTotal  uint64 `json:"rxTotal,omitempty"`  // 累计接收
	Conns    int64  `json:"conns,omitempty"`    // 活跃连接
	TotalConns uint64 `json:"totalConns,omitempty"` // 累计连接

	Message string `json:"message,omitempty"` // log 事件
	Time    time.Time `json:"time"`
}

// bus 是简单扇出事件总线：订阅者各自持有带缓冲 channel，
// 发送非阻塞，缓冲满则丢弃（UI 场景下宁可丢帧也不阻塞 core）。
type bus struct {
	mu   sync.RWMutex
	subs map[int]chan Event
	next int
}

func newBus() *bus {
	return &bus{subs: make(map[int]chan Event)}
}

func (b *bus) subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 128)
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = ch
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		if _, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

func (b *bus) emit(e Event) {
	e.Time = time.Now()
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default: // 订阅者消费慢：丢弃本条，保住发布方不阻塞
		}
	}
}

// Subscribe 返回事件通道与取消函数。
func (s *Supervisor) Subscribe() (<-chan Event, func()) {
	return s.events.subscribe()
}
