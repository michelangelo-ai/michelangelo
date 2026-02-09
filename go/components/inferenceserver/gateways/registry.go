package gateways

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type registry struct {
	backends map[v2pb.BackendType]backends.Backend
}

func newRegistry(backends map[v2pb.BackendType]backends.Backend) *registry {
	registry := &registry{
		backends: backends,
	}
	return registry
}

func (r *registry) getBackend(backendType v2pb.BackendType) (backends.Backend, error) {
	if backend, exists := r.backends[backendType]; exists {
		return backend, nil
	}
	return nil, fmt.Errorf("backend not found for type: %v", backendType)
}
