package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// CRD-related metrics
	crdUnmarshalErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crd_unmarshal_errors_total",
			Help: "Total number of CRD unmarshal errors",
		},
		[]string{"crd_type", "namespace", "error_type"},
	)

	// Controller reconciliation metrics
	reconciliationAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "controller_reconciliation_attempts_total",
			Help: "Total number of reconciliation attempts",
		},
		[]string{"controller", "namespace", "result"},
	)

	reconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "controller_reconciliation_duration_seconds",
			Help: "Duration of reconciliation operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller", "namespace"},
	)

	// Pipeline-related metrics
	activePipelines = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_pipelines_total",
			Help: "Current number of active pipelines",
		},
		[]string{"namespace", "status"},
	)

	pipelineCreations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_creations_total",
			Help: "Total number of pipeline creations",
		},
		[]string{"namespace", "pipeline_type"},
	)

	// Job-related metrics
	activeJobs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_jobs_total",
			Help: "Current number of active jobs",
		},
		[]string{"namespace", "job_type", "status"},
	)

	jobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "job_duration_seconds",
			Help: "Duration of job execution in seconds",
			Buckets: []float64{1, 10, 30, 60, 300, 600, 1800, 3600, 7200, 14400},
		},
		[]string{"namespace", "job_type"},
	)
)


// RegisterMetrics registers all metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		crdUnmarshalErrors,
		reconciliationAttempts,
		reconciliationDuration,
		activePipelines,
		pipelineCreations,
		activeJobs,
		jobDuration,
	)
}

// Metric accessor functions for direct use by controllers

// IncCRDUnmarshalError increments the CRD unmarshal error counter
func IncCRDUnmarshalError(crdType, namespace, errorType string) {
	crdUnmarshalErrors.WithLabelValues(crdType, namespace, errorType).Inc()
}

// IncReconciliationAttempt increments the reconciliation attempt counter
func IncReconciliationAttempt(controller, namespace, result string) {
	reconciliationAttempts.WithLabelValues(controller, namespace, result).Inc()
}

// ObserveReconciliationDuration records the duration of a reconciliation operation
func ObserveReconciliationDuration(controller, namespace string, duration float64) {
	reconciliationDuration.WithLabelValues(controller, namespace).Observe(duration)
}

// SetActivePipelines sets the number of active pipelines
func SetActivePipelines(namespace, status string, count float64) {
	activePipelines.WithLabelValues(namespace, status).Set(count)
}

// IncPipelineCreation increments the pipeline creation counter
func IncPipelineCreation(namespace, pipelineType string) {
	pipelineCreations.WithLabelValues(namespace, pipelineType).Inc()
}

// SetActiveJobs sets the number of active jobs
func SetActiveJobs(namespace, jobType, status string, count float64) {
	activeJobs.WithLabelValues(namespace, jobType, status).Set(count)
}

// ObserveJobDuration records the duration of a job execution
func ObserveJobDuration(namespace, jobType string, duration float64) {
	jobDuration.WithLabelValues(namespace, jobType).Observe(duration)
}