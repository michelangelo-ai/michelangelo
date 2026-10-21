package cluster

import (
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
)

// ReconcilerOption configures an optional dependency of the Reconciler.
type ReconcilerOption func(*Reconciler)

// WithSchedulerConfig sets the scheduler configuration. Under the Kueue
// backend, a dispatch that keeps failing without having reached its target
// is re-placed instead of failed (RFC 0f).
func WithSchedulerConfig(config maconfig.SchedulerConfig) ReconcilerOption {
	return func(r *Reconciler) {
		r.schedulerConfig = config
	}
}

// WithClusterReadiness sets the readiness view of registered clusters, which
// tells a cluster that was never probed apart from one that is not Ready.
// Without it, a registered target counts as Ready.
func WithClusterReadiness(readiness jobscluster.ClusterReadiness) ReconcilerOption {
	return func(r *Reconciler) {
		r.clusterReadiness = readiness
	}
}
