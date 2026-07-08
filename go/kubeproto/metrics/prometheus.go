package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	crReflectorErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cr_reflector_errors_total",
			Help: "Total number of reflector errors when listing or watching CRs",
		},
		[]string{"crd_type", "error_type", "blocking"},
	)
)

// RegisterMetrics registers all metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		crReflectorErrors,
	)
}

// IncCRUnmarshalError increments the CRD reflector error counter
func IncCRUnmarshalError(crdType, errorType, blocking string) {
	crReflectorErrors.WithLabelValues(crdType, errorType, blocking).Inc()
}
