package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// CR-related metrics
	crUnmarshalErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cr_unmarshal_errors_total",
			Help: "Total number of CR unmarshal errors",
		},
		[]string{"crd_type", "namespace", "error_type"},
	)
)

// RegisterMetrics registers all metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		crUnmarshalErrors,
	)
}

// Metric accessor functions for direct use by controllers

// IncCRUnmarshalError increments the CRD unmarshal error counter
func IncCRUnmarshalError(crdType, namespace, errorType string) {
	crUnmarshalErrors.WithLabelValues(crdType, namespace, errorType).Inc()
}
