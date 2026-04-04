package common

import (
	"sync"
)

type KVEntry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type Store struct {
	mu      sync.RWMutex
	data    map[string]KVEntry
	counter int64 // global version counter
}

func NewStore() *Store {
	return &Store{
		data: make(map[string]KVEntry),
	}
}

// Set writes a key-value pair and returns the new version.
func (s *Store) Set(key, value string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	s.data[key] = KVEntry{Value: value, Version: s.counter}
	return s.counter
}

// SetWithVersion writes a key-value pair with a specific version (used for replication).
// Only updates if the incoming version is newer than the current one.
func (s *Store) SetWithVersion(key, value string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.data[key]
	if !ok || version > existing.Version {
		s.data[key] = KVEntry{Value: value, Version: version}
	}
}

// Get returns the entry for a key and whether it exists.
func (s *Store) Get(key string) (KVEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.data[key]
	return entry, ok
}
