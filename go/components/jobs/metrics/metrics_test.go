package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
)

func TestJCMetricsConstructor(t *testing.T) {
	controllerMetrics := NewControllerMetrics(tally.NoopScope, "rayjob")
	require.NotNil(t, controllerMetrics)
}

func TestControllerMetrics_IncJobFailure(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{
			name:   "inc job failure count with reason",
			reason: "test_reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testScope := tally.NewTestScope("test", map[string]string{})

			m := ControllerMetrics{
				MetricsScope: testScope,
			}
			m.IncJobFailure(tt.reason)
			ta := testScope.Snapshot().Counters()
			require.Equal(t, ta["test.failed_count+failure_reason=test_reason"].Value(), int64(1))
		})
	}
}
