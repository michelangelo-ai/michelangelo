package client

import (
	"testing"
	"time"

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

// histogramOf reads a histogram from the controller-runtime registry.
func histogramOf(t *testing.T, name string, labels map[string]string) (count uint64, sum float64, upperBounds []float64) {
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
				for _, bucket := range metric.GetHistogram().GetBucket() {
					upperBounds = append(upperBounds, bucket.GetUpperBound())
				}
				return metric.GetHistogram().GetSampleCount(), metric.GetHistogram().GetSampleSum(), upperBounds
			}
		}
	}
	return 0, 0, nil
}

func TestAdmissionMetrics(t *testing.T) {
	RegisterDispatchMetrics()
	jobType := map[string]string{"job_type": "RayCluster"}

	before := counterValue(t, "michelangelo_jobs_admission_timeouts_total", jobType)
	IncAdmissionTimeouts("RayCluster")
	assert.Equal(t, before+1, counterValue(t, "michelangelo_jobs_admission_timeouts_total", jobType))

	count, sum, _ := histogramOf(t, "michelangelo_jobs_admission_wait_seconds", jobType)
	ObserveAdmissionWait("RayCluster", 90*time.Second)
	gotCount, gotSum, upperBounds := histogramOf(t, "michelangelo_jobs_admission_wait_seconds", jobType)
	assert.Equal(t, count+1, gotCount)
	assert.InDelta(t, sum+90, gotSum, 1e-9)

	// Exponential buckets from 1s to 36h.
	require.Len(t, upperBounds, 18)
	assert.InDelta(t, 1, upperBounds[0], 1e-9)
	assert.InDelta(t, (36 * time.Hour).Seconds(), upperBounds[len(upperBounds)-1], 1e-6)
	for i := 2; i < len(upperBounds); i++ {
		assert.InDelta(t, upperBounds[1]/upperBounds[0], upperBounds[i]/upperBounds[i-1], 1e-9, "bucket %d", i)
	}
}
