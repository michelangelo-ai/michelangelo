package handler

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
)

// MetricsHandlerImpl implements MetricsHandler interface.
// Focuses only on metrics and observability, following Flyte's metrics separation pattern.
type MetricsHandlerImpl struct {
	scope  tally.Scope
	logger logr.Logger
}

// NewMetricsHandler creates a new MetricsHandler implementation.
func NewMetricsHandler(scope tally.Scope, logger logr.Logger) MetricsHandler {
	return &MetricsHandlerImpl{
		scope:  scope,
		logger: logger.WithName("metrics-handler"),
	}
}

// RecordAPILatency records the latency of an API operation.
func (m *MetricsHandlerImpl) RecordAPILatency(operation string, duration float64, labels map[string]string) {
	m.logger.V(3).Info("Recording API latency",
		"operation", operation,
		"duration_ms", duration,
		"labels", labels,
	)

	// Create tagged scope with labels
	taggedScope := m.scope
	for key, value := range labels {
		taggedScope = taggedScope.Tagged(map[string]string{key: value})
	}

	// Record latency metric
	timer := taggedScope.Timer("api_action_latency")
	timer.Record(time.Duration(duration * float64(time.Millisecond)))

	// Also record as histogram for percentile analysis
	histogram := taggedScope.Histogram("api_action_latency_histogram", tally.DefaultBuckets)
	histogram.RecordDuration(time.Duration(duration * float64(time.Millisecond)))
}

// RecordAPIError records an API error.
func (m *MetricsHandlerImpl) RecordAPIError(operation string, errorCode string, labels map[string]string) {
	m.logger.V(2).Info("Recording API error",
		"operation", operation,
		"error_code", errorCode,
		"labels", labels,
	)

	// Create tagged scope with labels and error information
	tags := map[string]string{
		"operation":  operation,
		"error_code": errorCode,
	}
	for key, value := range labels {
		tags[key] = value
	}

	taggedScope := m.scope.Tagged(tags)

	// Record error counter
	counter := taggedScope.Counter("api_errors_total")
	counter.Inc(1)
}

// RecordStorageOperation records storage operation metrics.
func (m *MetricsHandlerImpl) RecordStorageOperation(storageType string, operation string, duration float64) {
	m.logger.V(3).Info("Recording storage operation",
		"storage_type", storageType,
		"operation", operation,
		"duration_ms", duration,
	)

	tags := map[string]string{
		"storage_type": storageType,
		"operation":    operation,
	}

	taggedScope := m.scope.Tagged(tags)

	// Record storage operation latency
	timer := taggedScope.Timer("storage_operation_latency")
	timer.Record(time.Duration(duration * float64(time.Millisecond)))

	// Record operation counter
	counter := taggedScope.Counter("storage_operations_total")
	counter.Inc(1)
}

// NullMetricsHandler is a no-op implementation for when metrics are disabled.
type NullMetricsHandler struct {
	logger logr.Logger
}

// NewNullMetricsHandler creates a no-op metrics handler.
func NewNullMetricsHandler(logger logr.Logger) MetricsHandler {
	return &NullMetricsHandler{
		logger: logger.WithName("null-metrics-handler"),
	}
}

// RecordAPILatency is a no-op for null handler.
func (n *NullMetricsHandler) RecordAPILatency(operation string, duration float64, labels map[string]string) {
	n.logger.V(3).Info("Metrics disabled, skipping API latency recording",
		"operation", operation,
		"duration_ms", duration,
	)
}

// RecordAPIError is a no-op for null handler.
func (n *NullMetricsHandler) RecordAPIError(operation string, errorCode string, labels map[string]string) {
	n.logger.V(3).Info("Metrics disabled, skipping API error recording",
		"operation", operation,
		"error_code", errorCode,
	)
}

// RecordStorageOperation is a no-op for null handler.
func (n *NullMetricsHandler) RecordStorageOperation(storageType string, operation string, duration float64) {
	n.logger.V(3).Info("Metrics disabled, skipping storage operation recording",
		"storage_type", storageType,
		"operation", operation,
		"duration_ms", duration,
	)
}