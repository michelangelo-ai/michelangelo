// Package keyedmutex provides a per-key mutex map for serializing access to
// shared resources identified by string keys.
package keyedmutex

import "sync"

// Map serializes operations on resources identified by string keys. Different
// keys lock independently; repeated calls with the same key return the same
// underlying mutex so callers always serialize against the same prior holder.
type Map struct {
	mu      sync.Mutex
	mutexes map[string]*sync.Mutex
}

// New returns an empty Map.
func New() *Map {
	return &Map{mutexes: make(map[string]*sync.Mutex)}
}

// Lock acquires the mutex for the given key, creating it on first use, and
// returns a function that releases it.
func (m *Map) Lock(key string) func() {
	m.mu.Lock()
	mutex, ok := m.mutexes[key]
	if !ok {
		mutex = &sync.Mutex{}
		m.mutexes[key] = mutex
	}
	m.mu.Unlock()

	mutex.Lock()
	return mutex.Unlock
}
