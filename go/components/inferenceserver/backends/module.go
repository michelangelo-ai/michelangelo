package backends

import (
	"go.uber.org/fx"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Module = fx.Options(
	fx.Provide(NewBackendRegistry),
)

// NewBackendRegistry creates and populates a backend registry with default backends.
func NewBackendRegistry() *Registry {
	registry := NewRegistry()

	// Register default backends
	registry.Register(v2pb.BACKEND_TYPE_TRITON, NewTritonBackend())

	return registry
}
