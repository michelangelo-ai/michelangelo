package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetControllerConfig(t *testing.T) {
	tests := []struct {
		name                 string
		config               IngesterConfig
		crdKind              string
		expectedConcurrency  int
		expectedRequeuePeriod time.Duration
	}{
		{
			name: "map override for concurrency only",
			config: IngesterConfig{
				ConcurrentReconciles: 2,
				RequeuePeriod:        30 * time.Second,
				ConcurrentReconcilesMap: map[string]int{
					"PipelineRun": 10,
				},
			},
			crdKind:              "PipelineRun",
			expectedConcurrency:  10,
			expectedRequeuePeriod: 30 * time.Second,
		},
		{
			name: "map override for both concurrency and requeue",
			config: IngesterConfig{
				ConcurrentReconciles: 2,
				RequeuePeriod:        30 * time.Second,
				ConcurrentReconcilesMap: map[string]int{
					"Deployment": 3,
				},
				RequeuePeriodMap: map[string]time.Duration{
					"Deployment": 60 * time.Second,
				},
			},
			crdKind:              "Deployment",
			expectedConcurrency:  3,
			expectedRequeuePeriod: 60 * time.Second,
		},
		{
			name: "fallback to legacy defaults",
			config: IngesterConfig{
				ConcurrentReconciles: 2,
				RequeuePeriod:        30 * time.Second,
				ConcurrentReconcilesMap: map[string]int{
					"PipelineRun": 10,
				},
			},
			crdKind:              "Model",
			expectedConcurrency:  2,
			expectedRequeuePeriod: 30 * time.Second,
		},
		{
			name: "no map, use legacy only",
			config: IngesterConfig{
				ConcurrentReconciles: 5,
				RequeuePeriod:        45 * time.Second,
			},
			crdKind:              "Pipeline",
			expectedConcurrency:  5,
			expectedRequeuePeriod: 45 * time.Second,
		},
		{
			name: "empty config returns zero values",
			config: IngesterConfig{},
			crdKind:              "Revision",
			expectedConcurrency:  0,
			expectedRequeuePeriod: 0,
		},
		{
			name: "map has zero value",
			config: IngesterConfig{
				ConcurrentReconciles: 2,
				ConcurrentReconcilesMap: map[string]int{
					"Model": 0, // Explicitly set to 0
				},
			},
			crdKind:             "Model",
			expectedConcurrency: 0, // Should use map value even if 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetControllerConfig(tt.crdKind)
			assert.Equal(t, tt.expectedConcurrency, result.ConcurrentReconciles,
				"ConcurrentReconciles mismatch")
			assert.Equal(t, tt.expectedRequeuePeriod, result.RequeuePeriod,
				"RequeuePeriod mismatch")
		})
	}
}

func TestGetControllerConfig_AllCRDTypes(t *testing.T) {
	// Test with a realistic config matching internal Uber setup
	config := IngesterConfig{
		ConcurrentReconciles: 2, // Default
		RequeuePeriod:        30 * time.Second,
		ConcurrentReconcilesMap: map[string]int{
			"PipelineRun": 10,
			"Deployment":  3,
			"Pipeline":    3,
			"Revision":    3,
		},
	}

	tests := []struct {
		crdKind             string
		expectedConcurrency int
	}{
		{"PipelineRun", 10},
		{"Deployment", 3},
		{"Pipeline", 3},
		{"Revision", 3},
		{"Model", 2},           // Falls back to default
		{"ModelFamily", 2},     // Falls back to default
		{"InferenceServer", 2}, // Falls back to default
		{"Project", 2},         // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.crdKind, func(t *testing.T) {
			result := config.GetControllerConfig(tt.crdKind)
			assert.Equal(t, tt.expectedConcurrency, result.ConcurrentReconciles)
			assert.Equal(t, 30*time.Second, result.RequeuePeriod)
		})
	}
}

func TestToIngesterConfig_BackwardsCompatibility(t *testing.T) {
	// Test that legacy ToIngesterConfig still works
	config := IngesterConfig{
		ConcurrentReconciles: 5,
		RequeuePeriod:        60 * time.Second,
	}

	result := config.ToIngesterConfig()
	assert.Equal(t, 5, result.ConcurrentReconciles)
	assert.Equal(t, 60*time.Second, result.RequeuePeriod)
}
