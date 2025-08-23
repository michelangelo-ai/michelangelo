package metrics

import (
	"sync"

	"github.com/uber-go/tally"
)

// SimpleMetricsCollector interface for basic metrics collection
type SimpleMetricsCollector interface {
	Increment(name string, tags map[string]string)
}

var (
	globalRegistry *Registry
	once           sync.Once
)

// Registry provides a global metrics registry for generated protobuf code
type Registry struct {
	scope     tally.Scope
	collector SimpleMetricsCollector
	mu        sync.RWMutex
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

// SetCollector sets the simple metrics collector
func (r *Registry) SetCollector(collector SimpleMetricsCollector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collector = collector
}

// IncrementCounter increments a counter with tags using the simple collector
func (r *Registry) IncrementCounter(name string, tags map[string]string) {
	r.mu.RLock()
	collector := r.collector
	r.mu.RUnlock()
	
	if collector != nil {
		collector.Increment(name, tags)
	}
}

// InitializeFromFX is called by FX to initialize the global registry with the injected scope
func InitializeFromFX(scope tally.Scope) {
	GetGlobalRegistry().SetScope(scope)
}

// InitializeCollector initializes the simple collector
func InitializeCollector(collector SimpleMetricsCollector) {
	GetGlobalRegistry().SetCollector(collector)
}