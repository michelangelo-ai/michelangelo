package kueue

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	decisions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "michelangelo_kueue_assignment_decisions_total",
			Help: "Total number of kueue_aware placement decisions, by strategy, the chosen tier and the outcome; " +
				"in shadow mode, actual_tier is the tier of the cluster_only pick that was assigned",
		},
		[]string{"strategy", "tier", "outcome", "actual_tier"},
	)

	registerDecisionMetricsOnce sync.Once
)

// RegisterDecisionMetrics registers the placement decision metric with the
// controller-runtime metrics registry, once.
func RegisterDecisionMetrics() {
	registerDecisionMetricsOnce.Do(func() {
		metrics.Registry.MustRegister(decisions)
	})
}

// countDecision counts a kueue_aware decision made under the strategy.
// actualTier is the tier of the cluster_only pick assigned in shadow mode,
// "none" when that pick is not a kueue_aware candidate, and empty for a
// decision that was not made in shadow mode.
func countDecision(strategy string, d Decision, actualTier string) {
	decisions.WithLabelValues(strategy, tierLabel(d.Tier), d.Outcome, actualTier).Inc()
}
