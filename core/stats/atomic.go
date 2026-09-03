package stats

import "sync/atomic"

// 轻量原子封装，避免在多个结构体上重复写样板。

type atomicUint64 struct{ v atomic.Uint64 }

func (a *atomicUint64) add(n uint64) { a.v.Add(n) }
func (a *atomicUint64) load() uint64 { return a.v.Load() }

type atomicInt64 struct{ v atomic.Int64 }

func (a *atomicInt64) add(n int64) { a.v.Add(n) }
func (a *atomicInt64) sub(n int64) { a.v.Add(-n) }
func (a *atomicInt64) load() int64 { return a.v.Load() }
