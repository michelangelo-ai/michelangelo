package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// CR-related metrics
	crdUnmarshalErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crd_unmarshal_errors_total",
			Help: "Total number of CR unmarshal errors",
		},
		[]string{"crd_type", "namespace", "error_type"},
	)
)

// RegisterMetrics registers all metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		crdUnmarshalErrors,
	)
}

// Metric accessor functions for direct use by controllers

// IncCRDUnmarshalError increments the CRD unmarshal error counter
func IncCRDUnmarshalError(crdType, namespace, errorType string) {
	crdUnmarshalErrors.WithLabelValues(crdType, namespace, errorType).Inc()
}
