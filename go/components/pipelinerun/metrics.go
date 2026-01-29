package pipelinerun

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// pipelineRunReconcileErrors tracks the total number of pipelinerun reconciliation errors
	pipelineRunReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_reconcile_errors_total",
			Help: "Total number of pipelinerun reconciliation errors",
		},
		[]string{"namespace", "pipeline_run"},
	)

	// pipelineRunReconcileSuccess tracks the total number of successful pipelinerun reconciliations
	pipelineRunReconcileSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_reconcile_success_total",
			Help: "Total number of successful pipelinerun reconciliations",
		},
		[]string{"namespace", "pipeline_run"},
	)

	// pipelineRunResult tracks pipelinerun results by state
	pipelineRunResult = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_result_total",
			Help: "Total number of pipelinerun results by state",
		},
		[]string{"namespace", "pipeline_run", "state", "pipeline_type", "environment", "tier"},
	)

	// pipelineRunResultSuccess tracks successful pipelinerun completions
	pipelineRunResultSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_result_success_total",
			Help: "Total number of successful pipelinerun completions",
		},
		[]string{"namespace", "pipeline_run", "pipeline_type", "environment", "tier", "region", "zone"},
	)

	// pipelineRunResultFailure tracks failed pipelinerun completions
	pipelineRunResultFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_result_failure_total",
			Help: "Total number of failed pipelinerun completions",
		},
		[]string{"namespace", "pipeline_run", "pipeline_type", "environment", "tier", "region", "zone", "failure_reason"},
	)

	// pipelineRunDuration tracks the duration of pipelinerun executions
	pipelineRunDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "pipelinerun_duration_seconds",
			Help: "Duration of pipelinerun executions in seconds",
			Buckets: []float64{
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
			},
		},
		[]string{"namespace", "pipeline_run", "state", "pipeline_type", "environment", "tier"},
	)

	// pipelineRunFailed is a gauge for tracking the most recent pipelinerun failure state (for alerting)
	pipelineRunFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pipelinerun_failed",
			Help: "Gauge indicating if the most recent pipelinerun failed (1) or succeeded (0), used for alerting",
		},
		[]string{"namespace", "pipeline_run", "pipeline_type", "environment", "zone"},
	)

	// pipelineRunStepSuccess tracks successful completions of individual pipeline steps
	pipelineRunStepSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipelinerun_step_success_total",
			Help: "Total number of successful pipeline step completions",
		},
		[]string{"namespace", "pipeline_run", "step_name", "pipeline_type"},
	)
)

// RegisterPipelineRunMetrics registers all pipelinerun metrics with the controller-runtime metrics registry
func RegisterPipelineRunMetrics() {
	metrics.Registry.MustRegister(
		pipelineRunReconcileErrors,
		pipelineRunReconcileSuccess,
		pipelineRunResult,
		pipelineRunResultSuccess,
		pipelineRunResultFailure,
		pipelineRunDuration,
		pipelineRunFailed,
		pipelineRunStepSuccess,
	)
}

// Metric accessor functions for direct use by the pipelinerun controller

// IncPipelineRunReconcileError increments the pipelinerun reconcile error counter
func IncPipelineRunReconcileError(namespace, pipelineRun string) {
	pipelineRunReconcileErrors.WithLabelValues(namespace, pipelineRun).Inc()
}

// IncPipelineRunReconcileSuccess increments the pipelinerun reconcile success counter
func IncPipelineRunReconcileSuccess(namespace, pipelineRun string) {
	pipelineRunReconcileSuccess.WithLabelValues(namespace, pipelineRun).Inc()
}

// PipelineRunMetricLabels contains labels for pipelinerun metrics
type PipelineRunMetricLabels struct {
	Namespace     string
	PipelineRun   string
	State         string
	PipelineType  string
	Environment   string
	Tier          string
	Region        string
	Zone          string
	FailureReason string
}

// IncPipelineRunResult increments the pipelinerun result counter with state
func IncPipelineRunResult(labels PipelineRunMetricLabels) {
	pipelineRunResult.WithLabelValues(
		labels.Namespace,
		labels.PipelineRun,
		labels.State,
		labels.PipelineType,
		labels.Environment,
		labels.Tier,
	).Inc()
}

// IncPipelineRunResultSuccess increments the pipelinerun success counter
func IncPipelineRunResultSuccess(labels PipelineRunMetricLabels) {
	pipelineRunResultSuccess.WithLabelValues(
		labels.Namespace,
		labels.PipelineRun,
		labels.PipelineType,
		labels.Environment,
		labels.Tier,
		labels.Region,
		labels.Zone,
	).Inc()
}

// IncPipelineRunResultFailure increments the pipelinerun failure counter
func IncPipelineRunResultFailure(labels PipelineRunMetricLabels) {
	pipelineRunResultFailure.WithLabelValues(
		labels.Namespace,
		labels.PipelineRun,
		labels.PipelineType,
		labels.Environment,
		labels.Tier,
		labels.Region,
		labels.Zone,
		labels.FailureReason,
	).Inc()
}

// ObservePipelineRunDuration records the duration of a pipelinerun
func ObservePipelineRunDuration(labels PipelineRunMetricLabels, duration time.Duration) {
	pipelineRunDuration.WithLabelValues(
		labels.Namespace,
		labels.PipelineRun,
		labels.State,
		labels.PipelineType,
		labels.Environment,
		labels.Tier,
	).Observe(duration.Seconds())
}

// SetPipelineRunFailed sets the pipelinerun failed gauge (1 for failed, 0 for succeeded)
func SetPipelineRunFailed(labels PipelineRunMetricLabels, failed bool) {
	value := 0.0
	if failed {
		value = 1.0
	}
	pipelineRunFailed.WithLabelValues(
		labels.Namespace,
		labels.PipelineRun,
		labels.PipelineType,
		labels.Environment,
		labels.Zone,
	).Set(value)
}

// IncPipelineRunStepSuccess increments the pipeline step success counter
func IncPipelineRunStepSuccess(namespace, pipelineRun, stepName, pipelineType string) {
	pipelineRunStepSuccess.WithLabelValues(namespace, pipelineRun, stepName, pipelineType).Inc()
}
