package config

// rebuildIndexes must be called with the lock already held (or during init).
func (s *Store) rebuildIndexes() {
	s.keyMap = make(map[string]struct{}, len(s.cfg.Keys))
	for _, k := range s.cfg.Keys {
		s.keyMap[k] = struct{}{}
	}
}
