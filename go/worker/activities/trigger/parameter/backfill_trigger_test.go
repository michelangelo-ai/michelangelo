package parameter

import (
	"testing"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillParameterGenerator_GenerateBatchParams(t *testing.T) {
	handler := &BackfillParameterGenerator{}

	// Create test timestamps - 2 days apart
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	startProto, _ := pbtypes.TimestampProto(startTime)
	endProto, _ := pbtypes.TimestampProto(endTime)

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
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}, // Daily at midnight
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]Params{
				{{Backfill: BackfillParam{}}}, // Single batch with empty param
			},
		},
		{
			name: "success - with parameters (single batch)",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}, // Daily at midnight
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {ParameterMap: map[string]string{"city": "nyc"}},
							"param2": {ParameterMap: map[string]string{"city": "sf"}},
						},
					},
				},
			},
			expectedError: false,
			// Will have 3 timestamps (day 1, 2, 3) * 2 params = 6 total params in one batch of 10
			expectedParams: [][]Params{
				{
					// Just verify structure - exact timestamps and order are tested in other tests
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
				},
			},
		},
		{
			name: "success - multiple batches with batch policy",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}, // Daily at midnight
						},
						BatchPolicy: &v2pb.BatchPolicy{BatchSize: 2},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {ParameterMap: map[string]string{"city": "nyc"}},
							"param2": {ParameterMap: map[string]string{"city": "sf"}},
						},
					},
				},
			},
			expectedError: false,
			// Will have 3 timestamps * 2 params = 6 total params, batch size 2 = 3 batches
			expectedParams: [][]Params{
				{
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
				},
				{
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
				},
				{
					{Backfill: BackfillParam{ParamID: "param1"}},
					{Backfill: BackfillParam{ParamID: "param2"}},
				},
			},
		},
		{
			name: "error - invalid start timestamp",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: nil,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
			},
			expectedError: true,
		},
		{
			name: "error - no schedule defined",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {},
						},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.GenerateBatchParams(tt.triggerRun)

			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Check structure matches
			assert.Len(t, result, len(tt.expectedParams), "Number of batches should match")

			for i, expectedBatch := range tt.expectedParams {
				if i < len(result) {
					assert.Len(t, result[i], len(expectedBatch), "Batch %d size should match", i)
					// Verify parameter IDs match (timestamps are auto-generated, so we skip exact comparison)
					for j, expectedParam := range expectedBatch {
						if j < len(result[i]) {
							if expectedParam.Backfill.ParamID != "" {
								assert.Equal(t, expectedParam.Backfill.ParamID, result[i][j].Backfill.ParamID)
								assert.NotNil(t, result[i][j].Backfill.ExecutionTimestamp, "ExecutionTimestamp should be set")
							}
						}
					}
				}
			}
		})
	}
}

func TestBackfillParameterGenerator_GenerateConcurrentParams(t *testing.T) {
	handler := &BackfillParameterGenerator{}

	// Create test timestamps
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	startProto, _ := pbtypes.TimestampProto(startTime)
	endProto, _ := pbtypes.TimestampProto(endTime)

	tests := []struct {
		name              string
		triggerRun        *v2pb.TriggerRun
		expectedError     bool
		expectedParamsLen int
	}{
		{
			name: "success - with parameters",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"}, // Daily
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {ParameterMap: map[string]string{"city": "nyc"}},
							"param2": {ParameterMap: map[string]string{"city": "sf"}},
						},
					},
				},
			},
			expectedError:     false,
			expectedParamsLen: 6, // 2 params * 3 daily timestamps
		},
		{
			name: "success - single parameter",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: startProto,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {},
						},
					},
				},
			},
			expectedError:     false,
			expectedParamsLen: 3, // 1 param * 3 daily timestamps
		},
		{
			name: "error - invalid timestamp",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: nil,
					EndTimestamp:   endProto,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.GenerateConcurrentParams(tt.triggerRun)

			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result, tt.expectedParamsLen)

			// Verify all params have execution timestamps and are sorted
			for i, param := range result {
				assert.NotNil(t, param.Backfill.ExecutionTimestamp)
				assert.NotEmpty(t, param.Backfill.ParamID)

				// Verify sorting: chronologically first, then alphabetically
				if i > 0 {
					prev := result[i-1]
					if prev.Backfill.ExecutionTimestamp.Equal(*param.Backfill.ExecutionTimestamp) {
						assert.LessOrEqual(t, prev.Backfill.ParamID, param.Backfill.ParamID)
					} else {
						assert.True(t, prev.Backfill.ExecutionTimestamp.Before(*param.Backfill.ExecutionTimestamp))
					}
				}
			}
		})
	}
}

func TestBackfillParameterGenerator_sortParams(t *testing.T) {
	time1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	time3 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		params   []Params
		expected []Params
	}{
		{
			name: "sort by timestamp then parameter ID",
			params: []Params{
				{Backfill: BackfillParam{ExecutionTimestamp: &time2, ParamID: "param2"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param2"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param1"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time3, ParamID: "param1"}},
			},
			expected: []Params{
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param1"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param2"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time2, ParamID: "param2"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time3, ParamID: "param1"}},
			},
		},
		{
			name: "same timestamp different parameters",
			params: []Params{
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param3"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param1"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param2"}},
			},
			expected: []Params{
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param1"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param2"}},
				{Backfill: BackfillParam{ExecutionTimestamp: &time1, ParamID: "param3"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortParams(tt.params)

			for i := range tt.params {
				assert.Equal(t, tt.expected[i].Backfill.ParamID, tt.params[i].Backfill.ParamID)
				assert.True(t, tt.expected[i].Backfill.ExecutionTimestamp.Equal(*tt.params[i].Backfill.ExecutionTimestamp))
			}
		})
	}
}

func TestBackfillParameterGenerator_GetParameterID(t *testing.T) {
	param := Params{
		Backfill: BackfillParam{
			ParamID: "test-param-id",
		},
	}

	result := param.GetParameterID()
	assert.Equal(t, "test-param-id", result)
}

func TestBackfillParameterGenerator_GetExecutionTimestamp(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	logicalTime := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC) // Different time

	param := Params{
		Backfill: BackfillParam{
			ExecutionTimestamp: &testTime,
		},
	}

	result := param.GetExecutionTimestamp(logicalTime)
	// Should return the execution timestamp from BackfillParam, not the logicalTs
	assert.Equal(t, testTime, result)
}

func TestBackfillParameterGenerator_GetTriggeredRun(t *testing.T) {
	execTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	createdTime := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

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
				Backfill: BackfillParam{
					ParamID:            "param1",
					ExecutionTimestamp: &execTime,
				},
			},
			pipelineRun: "pipeline-run-1",
			executionTs: execTime,
			createdAt:   createdTime,
			expectedList: []TriggeredRun{
				{
					ParamID:            "param1",
					PipelineRunName:    "pipeline-run-1",
					ExecutionTimestamp: execTime,
					CreatedAt:          createdTime,
					TriggerType:        "backfill",
				},
			},
		},
		{
			name: "append to existing list",
			initialContext: Object{
				"TriggeredRuns": []TriggeredRun{
					{
						ParamID:         "param1",
						PipelineRunName: "pipeline-run-1",
						TriggerType:     "backfill",
					},
				},
			},
			param: Params{
				Backfill: BackfillParam{
					ParamID:            "param2",
					ExecutionTimestamp: &execTime,
				},
			},
			pipelineRun: "pipeline-run-2",
			executionTs: execTime,
			createdAt:   createdTime,
			expectedList: []TriggeredRun{
				{
					ParamID:         "param1",
					PipelineRunName: "pipeline-run-1",
					TriggerType:     "backfill",
				},
				{
					ParamID:            "param2",
					PipelineRunName:    "pipeline-run-2",
					ExecutionTimestamp: execTime,
					CreatedAt:          createdTime,
					TriggerType:        "backfill",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get triggered run and append to list
			info := tt.param.GetTriggeredRun(tt.pipelineRun, tt.executionTs, tt.createdAt)
			tt.initialContext["TriggeredRuns"] = append(tt.initialContext["TriggeredRuns"].([]TriggeredRun), info)

			runs := tt.initialContext["TriggeredRuns"].([]TriggeredRun)
			assert.Equal(t, tt.expectedList, runs)
		})
	}
}

func TestCalculateBackfillRunTimestamps(t *testing.T) {
	tests := []struct {
		name               string
		cronSchedule       string
		startTimestamp     *pbtypes.Timestamp
		endTimestamp       *pbtypes.Timestamp
		expectedError      bool
		expectedTimestamps []time.Time
	}{
		{
			name:          "invalid cron expression",
			cronSchedule:  "invalid",
			expectedError: true,
		},
		{
			name:           "normal - daily schedule",
			cronSchedule:   "0 8 * * *",
			startTimestamp: &pbtypes.Timestamp{Seconds: 1698044400}, // 2023-10-23 07:00:00 UTC
			endTimestamp:   &pbtypes.Timestamp{Seconds: 1698217200}, // 2023-10-25 07:00:00 UTC
			expectedError:  false,
			expectedTimestamps: []time.Time{
				time.Date(2023, 10, 23, 8, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 24, 8, 0, 0, 0, time.UTC),
			},
		},
		{
			name:           "start timestamp is run",
			cronSchedule:   "0 7 * * *",
			startTimestamp: &pbtypes.Timestamp{Seconds: 1698044400},
			endTimestamp:   &pbtypes.Timestamp{Seconds: 1698235200},
			expectedError:  false,
			expectedTimestamps: []time.Time{
				time.Date(2023, 10, 23, 7, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 24, 7, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 25, 7, 0, 0, 0, time.UTC),
			},
		},
		{
			name:           "end timestamp is run",
			cronSchedule:   "0 12 * * *",
			startTimestamp: &pbtypes.Timestamp{Seconds: 1698044400},
			endTimestamp:   &pbtypes.Timestamp{Seconds: 1698235200},
			expectedError:  false,
			expectedTimestamps: []time.Time{
				time.Date(2023, 10, 23, 12, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 24, 12, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 25, 12, 0, 0, 0, time.UTC),
			},
		},
		{
			name:           "start and end timestamps are runs",
			cronSchedule:   "0 7 * * *",
			startTimestamp: &pbtypes.Timestamp{Seconds: 1698044400},
			endTimestamp:   &pbtypes.Timestamp{Seconds: 1698217200},
			expectedError:  false,
			expectedTimestamps: []time.Time{
				time.Date(2023, 10, 23, 7, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 24, 7, 0, 0, 0, time.UTC),
				time.Date(2023, 10, 25, 7, 0, 0, 0, time.UTC),
			},
		},
		{
			name:           "start timestamp equals end timestamp with run",
			cronSchedule:   "0 7 * * *",
			startTimestamp: &pbtypes.Timestamp{Seconds: 1698217200},
			endTimestamp:   &pbtypes.Timestamp{Seconds: 1698217200},
			expectedError:  false,
			expectedTimestamps: []time.Time{
				time.Date(2023, 10, 25, 7, 0, 0, 0, time.UTC),
			},
		},
		{
			name:               "start timestamp equals end timestamp without run",
			cronSchedule:       "0 8 * * *",
			startTimestamp:     &pbtypes.Timestamp{Seconds: 1698217200},
			endTimestamp:       &pbtypes.Timestamp{Seconds: 1698217200},
			expectedError:      false,
			expectedTimestamps: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a TriggerRun with the cron schedule
			tr := &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					StartTimestamp: test.startTimestamp,
					EndTimestamp:   test.endTimestamp,
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: test.cronSchedule},
						},
					},
				},
			}
			backfillRunTimestamps, err := calculateBackfillRunTimestamps(tr)
			if test.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expectedTimestamps, backfillRunTimestamps)
		})
	}
}
