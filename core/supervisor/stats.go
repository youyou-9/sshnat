package supervisor

import (
	"sync"
	"time"

	"github.com/sshnat/sshnat/core/stats"
)

// StartStatsTicker 每隔 interval 广播一次所有运行中隧道的流量增量事件，
// 返回停止函数。前端据此绘制 Tx/Rx 速率与 sparkline。
func (s *Supervisor) StartStatsTicker(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = time.Second
	}
	done := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		last := make(map[string]stats.Snapshot)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}

			s.mu.Lock()
			ids := make([]string, 0, len(s.tunnels))
			for id := range s.tunnels {
				ids = append(ids, id)
			}
			s.mu.Unlock()

			for _, id := range ids {
				snap := s.statsReg.Get(id).Snapshot()
				prev, seen := last[id]
				last[id] = snap
				if !seen {
					continue // 首个周期无增量可比
				}
				tx, rx := snap.Tx-prev.Tx, snap.Rx-prev.Rx
				if snap.Tx < prev.Tx {
					tx = 0
				}
				if snap.Rx < prev.Rx {
					rx = 0
				}
				s.events.emit(Event{
					Type:       EventStats,
					TunnelID:   id,
					Tx:         tx,
					Rx:         rx,
					TxTotal:    snap.Tx,
					RxTotal:    snap.Rx,
					Conns:      snap.ActiveConn,
					TotalConns: snap.TotalConn,
				})
			}

			// 清理已消失隧道的基线，防止 map 无限增长。
			if len(last) > len(ids)*2+16 {
				live := make(map[string]bool, len(ids))
				for _, id := range ids {
					live[id] = true
				}
				for id := range last {
					if !live[id] {
						delete(last, id)
					}
				}
			}
		}
	}()
	return func() { stopOnce.Do(func() { close(done) }) }
}
