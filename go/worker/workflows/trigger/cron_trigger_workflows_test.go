package trigger

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
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
			result, err := generatePipelineRunRequest(tt.triggerRun, tt.paramID, tt.pipelineRunName, tt.ts)

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
