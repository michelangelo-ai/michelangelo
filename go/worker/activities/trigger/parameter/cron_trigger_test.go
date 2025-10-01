package parameter

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronParameterGenerator_GenerateBatchParams(t *testing.T) {
	generator := &CronParameterGenerator{}

	tests := []struct {
		name           string
		triggerRun     *v2pb.TriggerRun
		expectedError  bool
		expectedParams [][]Params
	}{
		{
			name: "success - empty parameters map",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]Params{
				{{ParamID: ""}}, // Single batch with empty param
			},
		},
		{
			name: "success - with parameters map (single batch)",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"id1": {ParameterMap: map[string]string{"city": "los angelos"}},
							"id2": {ParameterMap: map[string]string{"city": "san jose"}},
							"id3": {ParameterMap: map[string]string{"city": "fremont"}},
						},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]Params{
				{ // Single batch with all 3 params
					{ParamID: "id1"},
					{ParamID: "id2"},
					{ParamID: "id3"},
				},
			},
		},
		{
			name: "success - multiple batches with batch size",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						BatchPolicy: &v2pb.BatchPolicy{BatchSize: 2}, // Force batch size of 2
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"id1": {ParameterMap: map[string]string{"city": "los angelos"}},
							"id2": {ParameterMap: map[string]string{"city": "san jose"}},
							"id3": {ParameterMap: map[string]string{"city": "fremont"}},
							"id4": {ParameterMap: map[string]string{"city": "san francisco"}},
							"id5": {ParameterMap: map[string]string{"city": "new york"}},
						},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]Params{
				{ // First batch (2 params)
					{ParamID: "id1"},
					{ParamID: "id2"},
				},
				{ // Second batch (2 params)
					{ParamID: "id3"},
					{ParamID: "id4"},
				},
				{ // Third batch (1 param)
					{ParamID: "id5"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateBatchParams(tt.triggerRun)

			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify the number of batches
			assert.Len(t, result, len(tt.expectedParams), "Number of batches should match")

			// Verify each batch individually
			for i, expectedParam := range tt.expectedParams {
				assert.ElementsMatch(t, expectedParam, result[i], "Batch %d should match exactly", i)
			}
		})
	}
}

func TestCronParameterGenerator_GenerateConcurrentParams(t *testing.T) {
	generator := &CronParameterGenerator{}

	tests := []struct {
		name              string
		triggerRun        *v2pb.TriggerRun
		expectedError     bool
		expectedParamsLen int
		expectedParams    []Params // Expected params in the result
	}{
		{
			name: "success - empty parameters map",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			expectedError:     false,
			expectedParamsLen: 0,
			expectedParams:    []Params{}, // Empty params
		},
		{
			name: "success - with parameters map",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"id1": {ParameterMap: map[string]string{"city": "los angelos"}},
							"id2": {ParameterMap: map[string]string{"city": "san jose"}},
						},
					},
				},
			},
			expectedError:     false,
			expectedParamsLen: 2,
			expectedParams: []Params{
				{ParamID: "id1"},
				{ParamID: "id2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateConcurrentParams(tt.triggerRun)

			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result, tt.expectedParamsLen)

			// Verify expected params match exactly if specified
			if len(tt.expectedParams) > 0 {
				assert.ElementsMatch(t, tt.expectedParams, result, "Params should match exactly")
			}
		})
	}
}
