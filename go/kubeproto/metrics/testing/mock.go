package testing

import (
	"sync"
)

// MockMetricsCollector provides an in-memory metrics collector for testing
// Following the pattern used in controller-runtime envtest
type MockMetricsCollector struct {
	mu      sync.RWMutex
	metrics map[string]float64
}

// NewMockMetricsCollector creates a new mock metrics collector for tests
func NewMockMetricsCollector() *MockMetricsCollector {
	return &MockMetricsCollector{
		metrics: make(map[string]float64),
	}
}

// Increment increments a metric counter for testing purposes
func (m *MockMetricsCollector) Increment(name string, tags map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a key from metric name and tags for test verification
	key := name
	for k, v := range tags {
		key += "_" + k + "_" + v
	}

	m.metrics[key]++
}

// GetMetrics returns all collected metrics for test verification
func (m *MockMetricsCollector) GetMetrics() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for k, v := range m.metrics {
		result[k] = v
	}
	return result
}

// GetMetric returns the value of a specific metric for assertions
func (m *MockMetricsCollector) GetMetric(name string, tags map[string]string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := name
	for k, v := range tags {
		key += "_" + k + "_" + v
	}

	return m.metrics[key]
}

// Reset clears all metrics for test isolation
func (m *MockMetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make(map[string]float64)
}
