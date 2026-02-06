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
)

// RegisterPipelineMetrics registers all pipeline metrics with the controller-runtime metrics registry
func RegisterPipelineMetrics() {
	metrics.Registry.MustRegister(
		pipelineReconcileErrors,
		pipelineReconcileSuccess,
		pipelineReady,
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
