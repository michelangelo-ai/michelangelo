package trigger

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePipelineRunRequest(t *testing.T) {
	tests := []struct {
		name                               string
		triggerRun                         *v2pb.TriggerRun
		paramID                            string
		pipelineRunName                    string
		ts                                 time.Time
		expectedError                      string
		expectedGeneratePipelineRunRequest *v2pb.CreatePipelineRunRequest
	}{
		{
			name: "Empty parameters",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-trigger",
				},
				Spec: v2pb.TriggerRunSpec{
					Pipeline: &api.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			paramID:         "",
			pipelineRunName: "test-pipeline-run-123",
			ts:              time.Date(2023, 1, 15, 10, 30, 45, 0, time.UTC),
			expectedGeneratePipelineRunRequest: &v2pb.CreatePipelineRunRequest{
				PipelineRun: &v2pb.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							ParameterIDLabel: "", // Empty when paramID is not in map
						},
					},
				},
			},
		},
		{
			name: "With parameters",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-trigger",
					Labels: map[string]string{
						EnvironmentLabel: "development",
					},
				},
				Spec: v2pb.TriggerRunSpec{
					Pipeline: &api.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {},
						},
					},
				},
			},
			paramID:         "param1",
			pipelineRunName: "test-pipeline-run-123",
			ts:              time.Date(2023, 1, 15, 10, 30, 45, 0, time.UTC),
			expectedGeneratePipelineRunRequest: &v2pb.CreatePipelineRunRequest{
				PipelineRun: &v2pb.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							ParameterIDLabel: "param1",
						},
					},
				},
			},
		},
		{
			name: "Invalid parameter ID",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-trigger",
				},
				Spec: v2pb.TriggerRunSpec{
					Pipeline: &api.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
					Trigger: &v2pb.Trigger{
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {},
						},
					},
				},
			},
			paramID:         "invalid-param",
			pipelineRunName: "test-run",
			ts:              time.Now(),
			expectedError:   "invalid parameter id: invalid-param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generatePipelineRunRequest(tt.triggerRun, tt.paramID, tt.pipelineRunName, tt.ts, nil)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)

				// Validate ParameterIDLabel
				expectedLabel := tt.expectedGeneratePipelineRunRequest.PipelineRun.ObjectMeta.Labels[ParameterIDLabel]
				actualLabel := result.PipelineRun.ObjectMeta.Labels[ParameterIDLabel]
				assert.Equal(t, expectedLabel, actualLabel)

				// Validate PipelineNameLabel
				assert.Equal(t, tt.triggerRun.Spec.Pipeline.Name, result.PipelineRun.ObjectMeta.Labels[PipelineNameLabel])
			}
		})
	}
}

func TestGenerateUniflowPRInput(t *testing.T) {
	tests := []struct {
		name           string
		params         *v2pb.PipelineExecutionParameters
		expectedResult *types.Struct
	}{
		{
			name: "Canvas flex - WorkflowConfig and TaskConfigs",
			params: &v2pb.PipelineExecutionParameters{
				WorkflowConfig: &types.Struct{
					Fields: map[string]*types.Value{
						"workflow_name": {Kind: &types.Value_StringValue{StringValue: "test-workflow"}},
						"version":       {Kind: &types.Value_NumberValue{NumberValue: 1.0}},
					},
				},
				TaskConfigs: map[string]*types.Struct{
					"task1": {
						Fields: map[string]*types.Value{
							"task_name": {Kind: &types.Value_StringValue{StringValue: "test-task-1"}},
						},
					},
					"task2": {
						Fields: map[string]*types.Value{
							"task_name": {Kind: &types.Value_StringValue{StringValue: "test-task-2"}},
						},
					},
				},
			},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{
					"workflow_config": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"workflow_name": {Kind: &types.Value_StringValue{StringValue: "test-workflow"}},
									"version":       {Kind: &types.Value_NumberValue{NumberValue: 1.0}},
								},
							},
						},
					},
					"task_configs": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"task1": {
										Kind: &types.Value_StructValue{
											StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													"task_name": {Kind: &types.Value_StringValue{StringValue: "test-task-1"}},
												},
											},
										},
									},
									"task2": {
										Kind: &types.Value_StructValue{
											StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													"task_name": {Kind: &types.Value_StringValue{StringValue: "test-task-2"}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Canvas flex - WorkflowConfig only",
			params: &v2pb.PipelineExecutionParameters{
				WorkflowConfig: &types.Struct{
					Fields: map[string]*types.Value{
						"workflow_name": {Kind: &types.Value_StringValue{StringValue: "test-workflow"}},
					},
				},
			},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{
					"workflow_config": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"workflow_name": {Kind: &types.Value_StringValue{StringValue: "test-workflow"}},
								},
							},
						},
					},
					"task_configs": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{Fields: map[string]*types.Value{}},
						},
					},
				},
			},
		},
		{
			name: "Canvas flex - TaskConfigs only",
			params: &v2pb.PipelineExecutionParameters{
				TaskConfigs: map[string]*types.Struct{
					"task1": {
						Fields: map[string]*types.Value{
							"task_type": {Kind: &types.Value_StringValue{StringValue: "preprocessing"}},
							"enabled":   {Kind: &types.Value_BoolValue{BoolValue: true}},
						},
					},
				},
			},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{
					"workflow_config": {
						Kind: &types.Value_StructValue{
							StructValue: nil,
						},
					},
					"task_configs": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"task1": {
										Kind: &types.Value_StructValue{
											StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													"task_type": {Kind: &types.Value_StringValue{StringValue: "preprocessing"}},
													"enabled":   {Kind: &types.Value_BoolValue{BoolValue: true}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Uniflow - Environ, Args",
			params: &v2pb.PipelineExecutionParameters{
				Environ: map[string]string{
					"ENV_VAR_1": "value1",
					"ENV_VAR_2": "value2",
				},
				Args: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							"arg_name": {Kind: &types.Value_StringValue{StringValue: "arg1"}},
						},
					},
					{
						Fields: map[string]*types.Value{
							"arg_name": {Kind: &types.Value_StringValue{StringValue: "arg2"}},
						},
					},
				},
			},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{
					"environ": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"ENV_VAR_1": {Kind: &types.Value_StringValue{StringValue: "value1"}},
									"ENV_VAR_2": {Kind: &types.Value_StringValue{StringValue: "value2"}},
								},
							},
						},
					},
					"args": {
						Kind: &types.Value_ListValue{
							ListValue: &types.ListValue{
								Values: []*types.Value{
									{
										Kind: &types.Value_StructValue{
											StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													"arg_name": {Kind: &types.Value_StringValue{StringValue: "arg1"}},
												},
											},
										},
									},
									{
										Kind: &types.Value_StructValue{
											StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													"arg_name": {Kind: &types.Value_StringValue{StringValue: "arg2"}},
												},
											},
										},
									},
								},
							},
						},
					},
					"kw_args": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{Fields: map[string]*types.Value{}},
						},
					},
				},
			},
		},
		{
			name: "Uniflow - Environ, KwArgs",
			params: &v2pb.PipelineExecutionParameters{
				Environ: map[string]string{
					"ENV_VAR_1": "value1",
					"ENV_VAR_2": "value2",
				},
				KwArgs: &types.Struct{
					Fields: map[string]*types.Value{
						"param_z": {Kind: &types.Value_StringValue{StringValue: "value_z"}},
						"param_a": {Kind: &types.Value_StringValue{StringValue: "value_a"}},
						"param_m": {Kind: &types.Value_NumberValue{NumberValue: 42.0}},
					},
				},
			},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{
					"environ": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"ENV_VAR_1": {Kind: &types.Value_StringValue{StringValue: "value1"}},
									"ENV_VAR_2": {Kind: &types.Value_StringValue{StringValue: "value2"}},
								},
							},
						},
					},
					"args": {
						Kind: &types.Value_ListValue{
							ListValue: &types.ListValue{Values: []*types.Value{}},
						},
					},
					"kw_args": {
						Kind: &types.Value_StructValue{
							StructValue: &types.Struct{
								Fields: map[string]*types.Value{
									"param_z": {Kind: &types.Value_StringValue{StringValue: "value_z"}},
									"param_a": {Kind: &types.Value_StringValue{StringValue: "value_a"}},
									"param_m": {Kind: &types.Value_NumberValue{NumberValue: 42.0}},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Empty parameters",
			params: &v2pb.PipelineExecutionParameters{},
			expectedResult: &types.Struct{
				Fields: map[string]*types.Value{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateUniflowPRInput(tt.params)

			// Compare protobuf structs directly
			assert.True(t, proto.Equal(tt.expectedResult, result))
		})
	}
}

// TODO(#564): Add comprehensive workflow execution tests with activity mocking once starlark-worker
// test framework supports Go workflows with Cadence/Temporal backend.
// Currently, starlark-worker's test suite does not support Go workflows using workflow.Context.
// The framework needs an ExecuteWorkflow() method that handles context wrapping for Go workflows.
func TestWorkflowsStruct(t *testing.T) {
	// Test that the workflows struct can be instantiated
	w := &workflows{}
	assert.NotNil(t, w)
}

func TestPrevScheduledTime(t *testing.T) {
	// anchor: 2024-01-15 10:00:00 UTC — a known cron firing time for hourly/daily schedules
	anchor := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		trigger    *v2pb.Trigger
		ts         time.Time
		wantNil    bool
		wantLastTs time.Time
	}{
		{
			name: "IntervalSchedule 1 hour — previous is exactly 1 hour before",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_IntervalSchedule{
					IntervalSchedule: &v2pb.IntervalSchedule{
						Interval: &types.Duration{Seconds: 3600},
					},
				},
			},
			ts:         anchor,
			wantLastTs: anchor.Add(-time.Hour),
		},
		{
			name: "IntervalSchedule 24 hours — previous is exactly 24 hours before",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_IntervalSchedule{
					IntervalSchedule: &v2pb.IntervalSchedule{
						Interval: &types.Duration{Seconds: 86400},
					},
				},
			},
			ts:         anchor,
			wantLastTs: anchor.Add(-24 * time.Hour),
		},
		{
			name: "CronSchedule hourly — previous hour",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "0 * * * *"},
				},
			},
			ts:         anchor,                 // 10:00
			wantLastTs: anchor.Add(-time.Hour), // 09:00
		},
		{
			name: "CronSchedule daily midnight — previous day",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
				},
			},
			ts:         time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), // midnight Jan 15
			wantLastTs: time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC), // midnight Jan 14
		},
		{
			name: "CronSchedule every 15 minutes — previous slot",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "*/15 * * * *"},
				},
			},
			ts:         time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			wantLastTs: time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
		},
		{
			name:    "nil trigger — returns nil",
			trigger: nil,
			ts:      anchor,
			wantNil: true,
		},
		{
			name: "BatchRerun trigger type — returns nil",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_BatchRerun{},
			},
			ts:      anchor,
			wantNil: true,
		},
		{
			name: "IntervalSchedule nil interval — returns nil",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_IntervalSchedule{
					IntervalSchedule: &v2pb.IntervalSchedule{Interval: nil},
				},
			},
			ts:      anchor,
			wantNil: true,
		},
		{
			name: "CronSchedule invalid expression — returns nil",
			trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "not-a-cron"},
				},
			},
			ts:      anchor,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prevScheduledTime(tt.trigger, tt.ts)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantLastTs.UTC(), got.UTC())
		})
	}
}

func TestGeneratePipelineRunRequestInjectsLastExecutionTimestamp(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	lastTs := ts.Add(-time.Hour)

	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "my-trigger",
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{Name: "my-pipeline", Namespace: "test-ns"},
			Trigger:  &v2pb.Trigger{},
		},
	}

	t.Run("lastTs injected into environ when provided", func(t *testing.T) {
		req, err := generatePipelineRunRequest(triggerRun, "", "run-1", ts, &lastTs)
		require.NoError(t, err)
		require.NotNil(t, req.PipelineRun.Spec.Input)

		environField := req.PipelineRun.Spec.Input.Fields["environ"]
		require.NotNil(t, environField, "environ field must be present")

		val := environField.GetStructValue().Fields["LAST_SCHEDULE_TIMESTAMP"]
		require.NotNil(t, val, "LAST_SCHEDULE_TIMESTAMP must be set in environ")
		assert.Equal(t, "1705309200", val.GetStringValue()) // lastTs unix = ts - 1h
	})

	t.Run("no environ injected when lastTs is nil", func(t *testing.T) {
		req, err := generatePipelineRunRequest(triggerRun, "", "run-2", ts, nil)
		require.NoError(t, err)
		// Input may be nil or environ may be absent — either is correct
		if req.PipelineRun.Spec.Input != nil {
			environField := req.PipelineRun.Spec.Input.Fields["environ"]
			if environField != nil {
				val := environField.GetStructValue().Fields["LAST_SCHEDULE_TIMESTAMP"]
				assert.Nil(t, val, "LAST_SCHEDULE_TIMESTAMP must not be set when lastTs is nil")
			}
		}
	})

	t.Run("lastTs merged into existing environ from ParametersMap", func(t *testing.T) {
		trWithParams := &v2pb.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-trigger"},
			Spec: v2pb.TriggerRunSpec{
				Pipeline: &api.ResourceIdentifier{Name: "my-pipeline", Namespace: "test-ns"},
				Trigger: &v2pb.Trigger{
					ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
						"default": {
							Environ: map[string]string{"MY_VAR": "my-value"},
						},
					},
				},
			},
		}
		req, err := generatePipelineRunRequest(trWithParams, "default", "run-3", ts, &lastTs)
		require.NoError(t, err)
		require.NotNil(t, req.PipelineRun.Spec.Input)

		environField := req.PipelineRun.Spec.Input.Fields["environ"]
		require.NotNil(t, environField)
		fields := environField.GetStructValue().Fields

		// Existing environ key preserved
		assert.Equal(t, "my-value", fields["MY_VAR"].GetStringValue())
		// New key injected
		assert.Equal(t, "1705309200", fields["LAST_SCHEDULE_TIMESTAMP"].GetStringValue())
	})
}
