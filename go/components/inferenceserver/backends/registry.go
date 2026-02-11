package backends

import (
	"fmt"
	"sync"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Registry manages backend implementations and provides lookup by type.
type Registry struct {
	mu       sync.RWMutex
	backends map[v2pb.BackendType]Backend
}

// NewRegistry creates an empty backend registry.
func NewRegistry() *Registry {
	return &Registry{
		backends: make(map[v2pb.BackendType]Backend),
	}
}

// Register adds a backend implementation for a specific type.
func (r *Registry) Register(backendType v2pb.BackendType, backend Backend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[backendType] = backend
}

// GetBackend retrieves a backend by type.
// Returns an error if no backend is registered for the given type.
func (r *Registry) GetBackend(backendType v2pb.BackendType) (Backend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if backend, exists := r.backends[backendType]; exists {
		return backend, nil
	}
	return nil, fmt.Errorf("backend not found for type: %v", backendType)
}
