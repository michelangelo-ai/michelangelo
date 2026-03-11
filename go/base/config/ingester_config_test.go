package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIngesterConfig_Fields(t *testing.T) {
	config := IngesterConfig{
		ConcurrentReconciles: 2,
		RequeuePeriod:        30 * time.Second,
		ConcurrentReconcilesMap: map[string]int{
			"PipelineRun": 10,
			"Deployment":  3,
		},
		RequeuePeriodMap: map[string]time.Duration{
			"Deployment": 60 * time.Second,
		},
	}

	assert.Equal(t, 2, config.ConcurrentReconciles)
	assert.Equal(t, 30*time.Second, config.RequeuePeriod)
	assert.Equal(t, 10, config.ConcurrentReconcilesMap["PipelineRun"])
	assert.Equal(t, 3, config.ConcurrentReconcilesMap["Deployment"])
	assert.Equal(t, 60*time.Second, config.RequeuePeriodMap["Deployment"])
}
