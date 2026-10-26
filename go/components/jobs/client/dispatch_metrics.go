package client

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Re-placement reasons (RFC 0f).
const (
	ReplacementReasonTargetNotReady = "target_not_ready"
	ReplacementReasonWindowExpired  = "window_expired"
)

// admissionWaitBuckets span admission waits from 1s to 36h (RFC 0a).
var admissionWaitBuckets = prometheus.ExponentialBucketsRange(1, (36 * time.Hour).Seconds(), 18)

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

	admissionTimeouts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "michelangelo_jobs_admission_timeouts_total",
			Help: "Total number of jobs failed with AdmissionTimeout because their wait for admission exceeded the bound",
		},
		[]string{"job_type"},
	)

	admissionWait = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "michelangelo_jobs_admission_wait_seconds",
			Help:    "Wait for admission of admitted jobs, from creation or their latest eviction, in seconds",
			Buckets: admissionWaitBuckets,
		},
		[]string{"job_type"},
	)

	registerDispatchMetricsOnce sync.Once
)

// RegisterDispatchMetrics registers the dispatch and admission metrics with
// the controller-runtime metrics registry. Both job controllers call it, so
// it registers once.
func RegisterDispatchMetrics() {
	registerDispatchMetricsOnce.Do(func() {
		metrics.Registry.MustRegister(
			createRetries,
			replacements,
			admissionTimeouts,
			admissionWait,
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

// IncAdmissionTimeouts increments the admission timeout counter.
func IncAdmissionTimeouts(jobType string) {
	admissionTimeouts.WithLabelValues(jobType).Inc()
}

// ObserveAdmissionWait records the wait of an admitted job.
func ObserveAdmissionWait(jobType string, wait time.Duration) {
	admissionWait.WithLabelValues(jobType).Observe(wait.Seconds())
}
