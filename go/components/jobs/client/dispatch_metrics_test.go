package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// counterValue reads a counter from the controller-runtime registry.
func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	families, err := metrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			matched := len(metric.GetLabel()) == len(labels)
			for _, label := range metric.GetLabel() {
				if labels[label.GetName()] != label.GetValue() {
					matched = false
				}
			}
			if matched {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestDispatchMetrics(t *testing.T) {
	RegisterDispatchMetrics()
	// Both job controllers register: the second call is a no-op.
	require.NotPanics(t, RegisterDispatchMetrics)

	retries := map[string]string{"job_type": "RayCluster", "class": "rejected"}
	before := counterValue(t, "michelangelo_jobs_create_retries_total", retries)
	IncCreateRetries("RayCluster", DispatchRejected)
	IncCreateRetries("RayCluster", DispatchRejected)
	assert.Equal(t, before+2, counterValue(t, "michelangelo_jobs_create_retries_total", retries))

	replaced := map[string]string{"job_type": "RayCluster", "reason": ReplacementReasonWindowExpired}
	before = counterValue(t, "michelangelo_jobs_replacements_total", replaced)
	IncReplacements("RayCluster", ReplacementReasonWindowExpired)
	assert.Equal(t, before+1, counterValue(t, "michelangelo_jobs_replacements_total", replaced))
}
