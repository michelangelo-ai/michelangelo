package cascadedelete

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestCascadeCollectorsRegister registers every cascade collector into a fresh,
// isolated registry to confirm they are well-formed and uniquely named (a nil
// collector or a duplicate metric name would error here). It mirrors what
// RegisterMetrics does against the controller-runtime registry, but without the
// global side effect.
//
// Note: counter *values* are deliberately not asserted here. The only zero-dep
// way to read a prometheus counter pulls in client_model / testutil, neither of
// which is wired into this repo's Bazel module graph; the increment call sites
// are instead exercised behaviorally by the RunDrainStep tests.
func TestCascadeCollectorsRegister(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, c := range []prometheus.Collector{
		ownerRefBackfillTotal,
		childDrainDuration,
		childDrainTimeoutTotal,
		childDrainActive,
	} {
		require.NoError(t, reg.Register(c))
	}
}

// TestCascadeMetricHelpersDoNotPanic exercises each increment/observe helper to
// confirm label routing is valid: a wrong label cardinality or a nil collector
// would panic on the call below.
func TestCascadeMetricHelpersDoNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		for _, kind := range []string{"pipeline_run", "trigger_run"} {
			IncOwnerRefBackfill(kind)
			IncDrainTimeout(kind)
			IncDrainActive(kind)
			DecDrainActive(kind)
			ObserveDrainDuration(kind, 1.0)
		}
	})
}
