package pipeline

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// pipelineReconcileErrors tracks the total number of pipeline reconciliation errors
	pipelineReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_reconcile_errors_total",
			Help: "Total number of pipeline reconciliation errors",
		},
		[]string{"namespace", "pipeline"},
	)

	// pipelineReconcileSuccess tracks the total number of successful pipeline reconciliations
	pipelineReconcileSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_reconcile_success_total",
			Help: "Total number of successful pipeline reconciliations",
		},
		[]string{"namespace", "pipeline"},
	)

	// pipelineReady tracks pipelines that have reached READY state
	pipelineReady = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_ready_total",
			Help: "Total number of pipelines that reached READY state",
		},
		[]string{"namespace", "pipeline", "pipeline_type"},
	)

	// pipelineCascadeDeleteStarted tracks cascade delete initiations
	pipelineCascadeDeleteStarted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_cascade_delete_started_total",
			Help: "Total number of pipeline cascade deletes started (one per cascade, not per requeue)",
		},
		[]string{"namespace", "pipeline"},
	)

	// pipelineCascadeDeleteCompleted tracks successful cascade deletes
	pipelineCascadeDeleteCompleted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_cascade_delete_completed_total",
			Help: "Total number of pipeline cascade deletes completed",
		},
		[]string{"namespace", "pipeline"},
	)

	// pipelineCascadeDeleteError tracks cascade delete errors by reason
	pipelineCascadeDeleteError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pipeline_cascade_delete_error_total",
			Help: "Total number of pipeline cascade delete errors, tagged by reason (list_error, delete_error, update_error, kill_timeout)",
		},
		[]string{"namespace", "pipeline", "reason"},
	)

	// pipelineCascadeDeleteActiveChildren tracks active children during cascade delete.
	// The `kind` label distinguishes between trigger_run and pipeline_run so dashboards
	// can visualize each independently instead of a toggling single value.
	pipelineCascadeDeleteActiveChildren = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pipeline_cascade_delete_active_children",
			Help: "Number of active children during pipeline cascade delete, broken down by kind",
		},
		[]string{"namespace", "pipeline", "kind"},
	)
)

// RegisterPipelineMetrics registers all pipeline metrics with the controller-runtime metrics registry
func RegisterPipelineMetrics() {
	metrics.Registry.MustRegister(
		pipelineReconcileErrors,
		pipelineReconcileSuccess,
		pipelineReady,
		pipelineCascadeDeleteStarted,
		pipelineCascadeDeleteCompleted,
		pipelineCascadeDeleteError,
		pipelineCascadeDeleteActiveChildren,
	)
}

// Metric accessor functions for direct use by the pipeline controller

// IncPipelineReconcileError increments the pipeline reconcile error counter
func IncPipelineReconcileError(namespace, pipeline string) {
	pipelineReconcileErrors.WithLabelValues(namespace, pipeline).Inc()
}

// IncPipelineReconcileSuccess increments the pipeline reconcile success counter
func IncPipelineReconcileSuccess(namespace, pipeline string) {
	pipelineReconcileSuccess.WithLabelValues(namespace, pipeline).Inc()
}

// IncPipelineReady increments the pipeline ready counter
func IncPipelineReady(namespace, pipeline, pipelineType string) {
	pipelineReady.WithLabelValues(namespace, pipeline, pipelineType).Inc()
}

// IncCascadeDeleteStarted increments the cascade delete started counter
func IncCascadeDeleteStarted(namespace, pipeline string) {
	pipelineCascadeDeleteStarted.WithLabelValues(namespace, pipeline).Inc()
}

// IncCascadeDeleteCompleted increments the cascade delete completed counter
func IncCascadeDeleteCompleted(namespace, pipeline string) {
	pipelineCascadeDeleteCompleted.WithLabelValues(namespace, pipeline).Inc()
}

// IncCascadeDeleteError increments the cascade delete error counter with a
// reason label (list_error, delete_error, update_error, kill_timeout).
func IncCascadeDeleteError(namespace, pipeline, reason string) {
	pipelineCascadeDeleteError.WithLabelValues(namespace, pipeline, reason).Inc()
}

// SetCascadeDeleteActiveChildren sets the active children gauge for a specific
// child kind (trigger_run or pipeline_run).
func SetCascadeDeleteActiveChildren(namespace, pipeline, kind string, count int) {
	pipelineCascadeDeleteActiveChildren.WithLabelValues(namespace, pipeline, kind).Set(float64(count))
}
