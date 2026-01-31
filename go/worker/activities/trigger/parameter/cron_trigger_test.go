package parameter

import (
	"testing"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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

func TestCronParameterGenerator_SortParams(t *testing.T) {
	tests := []struct {
		name     string
		params   []Params
		expected []Params
	}{
		{
			name: "sort by parameter ID alphabetically",
			params: []Params{
				{ParamID: "param3"},
				{ParamID: "param1"},
				{ParamID: "param2"},
			},
			expected: []Params{
				{ParamID: "param1"},
				{ParamID: "param2"},
				{ParamID: "param3"},
			},
		},
		{
			name: "already sorted",
			params: []Params{
				{ParamID: "a"},
				{ParamID: "b"},
				{ParamID: "c"},
			},
			expected: []Params{
				{ParamID: "a"},
				{ParamID: "b"},
				{ParamID: "c"},
			},
		},
		{
			name:     "empty params",
			params:   []Params{},
			expected: []Params{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortParams(tt.params)
			assert.Equal(t, tt.expected, tt.params)
		})
	}
}

func TestCronParameterGenerator_GetParameterID(t *testing.T) {
	param := Params{
		ParamID: "test-param-id",
	}

	result := param.GetParameterID()
	assert.Equal(t, "test-param-id", result)
}

func TestCronParameterGenerator_GetExecutionTimestamp(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	logicalTime := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)

	param := Params{
		ParamID: "test-param",
	}

	result := param.GetExecutionTimestamp(logicalTime)
	// For cron trigger, should return the logical timestamp
	assert.Equal(t, logicalTime, result)
	assert.NotEqual(t, testTime, result)
}
func TestCronParameterGenerator_GetTriggeredRun(t *testing.T) {
	createdTime := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	executionTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		initialContext Object
		param          Params
		pipelineRun    string
		executionTs    time.Time
		createdAt      time.Time
		expectedList   []TriggeredRun
	}{
		{
			name: "append to empty list",
			initialContext: Object{
				"TriggeredRuns": []TriggeredRun{},
			},
			param: Params{
				ParamID: "param1",
			},
			pipelineRun: "pipeline-run-1",
			executionTs: executionTime,
			createdAt:   createdTime,
			expectedList: []TriggeredRun{
				{
					ParamID:            "param1",
					PipelineRunName:    "pipeline-run-1",
					ExecutionTimestamp: executionTime,
					CreatedAt:          createdTime,
					TriggerType:        "cron",
				},
			},
		},
		{
			name: "append to existing list",
			initialContext: Object{
				"TriggeredRuns": []TriggeredRun{
					{
						ParamID:            "param1",
						PipelineRunName:    "pipeline-run-1",
						ExecutionTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						CreatedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						TriggerType:        "cron",
					},
				},
			},
			param: Params{
				ParamID: "param2",
			},
			pipelineRun: "pipeline-run-2",
			executionTs: executionTime,
			createdAt:   createdTime,
			expectedList: []TriggeredRun{
				{
					ParamID:            "param1",
					PipelineRunName:    "pipeline-run-1",
					ExecutionTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					CreatedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					TriggerType:        "cron",
				},
				{
					ParamID:            "param2",
					PipelineRunName:    "pipeline-run-2",
					ExecutionTimestamp: executionTime,
					CreatedAt:          createdTime,
					TriggerType:        "cron",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get triggered run and append to list
			info := tt.param.GetTriggeredRun(tt.pipelineRun, tt.executionTs, tt.createdAt)
			tt.initialContext["TriggeredRuns"] = append(tt.initialContext["TriggeredRuns"].([]TriggeredRun), info)

			triggeredRuns := tt.initialContext["TriggeredRuns"].([]TriggeredRun)
			assert.Equal(t, tt.expectedList, triggeredRuns)
		})
	}
}
