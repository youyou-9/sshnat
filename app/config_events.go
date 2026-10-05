package app

// emitConfigChanged notifies native UI consumers after a successful save.
// Callers release the configuration lock first so consumers can reload it.
// The event intentionally contains no configuration or credential payload.
func (s *Services) emitConfigChanged() {
	s.emu.RLock()
	emitter := s.emit
	s.emu.RUnlock()
	if emitter != nil {
		emitter(EventConfigChanged)
	}
}

// Shutdown serializes with Start so an already queued tray/RPC action cannot
// create a new run after the last StopAll has finished. Services itself is not
// an RPC service; only its Host/Tunnel/Settings projections are registered.
func (s *Services) Shutdown() {
	s.tmu.Lock()
	defer s.tmu.Unlock()
	s.closing = true
	s.Sup.StopAll()
}
