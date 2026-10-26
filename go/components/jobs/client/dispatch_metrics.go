package client

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Re-placement reasons (RFC 0f).
const (
	ReplacementReasonTargetNotReady = "target_not_ready"
	ReplacementReasonWindowExpired  = "window_expired"
)

var (
	createRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "michelangelo_jobs_create_retries_total",
			Help: "Total number of failed creates on compute clusters that will be retried, by error class",
		},
		[]string{"job_type", "class"},
	)

	replacements = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "michelangelo_jobs_replacements_total",
			Help: "Total number of jobs re-placed after failed creates on their target cluster",
		},
		[]string{"job_type", "reason"},
	)

	registerDispatchMetricsOnce sync.Once
)

// RegisterDispatchMetrics registers the dispatch metrics with the
// controller-runtime metrics registry. Both job controllers call it, so it
// registers once.
func RegisterDispatchMetrics() {
	registerDispatchMetricsOnce.Do(func() {
		metrics.Registry.MustRegister(
			createRetries,
			replacements,
		)
	})
}

// IncCreateRetries increments the create retry counter.
func IncCreateRetries(jobType string, class DispatchClass) {
	createRetries.WithLabelValues(jobType, class.String()).Inc()
}

// IncReplacements increments the re-placement counter.
func IncReplacements(jobType, reason string) {
	replacements.WithLabelValues(jobType, reason).Inc()
}
