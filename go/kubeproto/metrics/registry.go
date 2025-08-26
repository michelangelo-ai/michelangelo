package metrics

import (
	"sync"

	"github.com/uber-go/tally"
)

var (
	globalRegistry *Registry
	once           sync.Once
)

// Registry provides a global metrics registry for generated protobuf code
type Registry struct {
	scope tally.Scope
	mu    sync.RWMutex
}

// GetGlobalRegistry returns the singleton metrics registry
func GetGlobalRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			scope: tally.NoopScope, // Default to noop until initialized
		}
	})
	return globalRegistry
}

// SetScope sets the tally scope for the global registry
func (r *Registry) SetScope(scope tally.Scope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scope = scope
}

// GetScope returns the current tally scope
func (r *Registry) GetScope() tally.Scope {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.scope
}

// Counter returns a counter with the given name
func (r *Registry) Counter(name string) tally.Counter {
	return r.GetScope().Counter(name)
}

// Tagged returns a tagged scope
func (r *Registry) Tagged(tags map[string]string) tally.Scope {
	return r.GetScope().Tagged(tags)
}

// IncrementCounter increments a counter with tags using direct Prometheus metrics
func (r *Registry) IncrementCounter(name string, tags map[string]string) {
	// Delegate to Prometheus metrics based on the metric name
	switch name {
	case "crd_unmarshal_errors":
		IncCRDUnmarshalError(
			getTagValueOrDefault(tags, "resource_type"),
			getTagValueOrDefault(tags, "namespace"),
			getTagValueOrDefault(tags, "error_type"),
		)
	}
}

// InitializeFromFX is called by FX to initialize the global registry with the injected scope
func InitializeFromFX(scope tally.Scope) {
	GetGlobalRegistry().SetScope(scope)
}

// getTagValueOrDefault safely extracts a tag value or returns "unknown" if not present
func getTagValueOrDefault(tags map[string]string, key string) string {
	if value, ok := tags[key]; ok {
		return value
	}
	return "unknown"
}
