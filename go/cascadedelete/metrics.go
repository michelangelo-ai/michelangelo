package cascadedelete

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ownerRefBackfillTotal counts ownerReference backfills, incremented only on
// actual writes (not every reconcile), partitioned by child kind.
var ownerRefBackfillTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "cascade_owner_ref_backfill_total",
		Help: "Total number of ownerReference backfills performed, by child kind",
	},
	[]string{"kind"},
)

// drainDurationBuckets spans seconds to 24h so a child's drain time shares the
// same scale as a long-running workflow.
var drainDurationBuckets = []float64{
	30,    // 30 seconds
	60,    // 1 minute
	300,   // 5 minutes
	600,   // 10 minutes
	1800,  // 30 minutes
	3600,  // 1 hour
	7200,  // 2 hours
	14400, // 4 hours
	28800, // 8 hours
	86400, // 24 hours
}

// childDrainDuration records how long a child spent draining before its drain
// finalizer was removed, partitioned by child kind.
var childDrainDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "cascade_child_drain_duration_seconds",
		Help:    "Time a child spent draining before its drain finalizer was removed, by child kind",
		Buckets: drainDurationBuckets,
	},
	[]string{"kind"},
)

// childDrainTimeoutTotal counts children whose drain exceeded the safety timeout
// and were force-completed rather than draining cleanly.
var childDrainTimeoutTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "cascade_child_drain_timeout_total",
		Help: "Total number of children whose drain exceeded the safety timeout, by child kind",
	},
	[]string{"kind"},
)

// childDrainActive tracks the number of children currently draining, by child
// kind. Being a gauge, it is best-effort across controller restarts.
var childDrainActive = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "cascade_child_drain_active",
		Help: "Number of children currently draining, by child kind",
	},
	[]string{"kind"},
)

// RegisterMetrics registers the cascade metrics with the controller-runtime registry.
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		ownerRefBackfillTotal,
		childDrainDuration,
		childDrainTimeoutTotal,
		childDrainActive,
	)
}

// IncOwnerRefBackfill counts an ownerReference backfill for the given child kind.
func IncOwnerRefBackfill(kind string) {
	ownerRefBackfillTotal.WithLabelValues(kind).Inc()
}

// ObserveDrainDuration records a completed drain's duration in seconds, by kind.
func ObserveDrainDuration(kind string, seconds float64) {
	childDrainDuration.WithLabelValues(kind).Observe(seconds)
}

// IncDrainTimeout counts a child whose drain exceeded the safety timeout.
func IncDrainTimeout(kind string) {
	childDrainTimeoutTotal.WithLabelValues(kind).Inc()
}

// IncDrainActive bumps the active-drain gauge for the given child kind.
func IncDrainActive(kind string) {
	childDrainActive.WithLabelValues(kind).Inc()
}

// DecDrainActive lowers the active-drain gauge for the given child kind.
func DecDrainActive(kind string) {
	childDrainActive.WithLabelValues(kind).Dec()
}
