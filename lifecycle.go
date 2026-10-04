package main

import (
	"sync"

	"github.com/sshnat/sshnat/app"
)

// Tray native calls and Close run on the UI thread, so shutdown never waits
// for a worker that holds this mutex while awaiting an InvokeSync callback.
type nativeUpdateGate struct {
	mu     sync.Mutex
	closed bool
}

func (g *nativeUpdateGate) Run(update func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed {
		update()
	}
}

func (g *nativeUpdateGate) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

func (g *nativeUpdateGate) Closed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

func makeShutdownHandler(services *app.Services, stopStats, closeNativeUpdates func()) func() {
	return sync.OnceFunc(func() {
		closeNativeUpdates()
		stopStats()
		services.SetEmitter(nil)
		services.Shutdown()
	})
}
