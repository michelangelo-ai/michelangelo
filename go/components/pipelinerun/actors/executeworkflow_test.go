package actors

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	blobstoreMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowclientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/require"
	uberconfig "go.uber.org/config"
	"go.uber.org/zap/zaptest"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestExecuteWorkflowActor(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)
	pipelineManifestContet := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
	}

	// Create previous successful pipeline runs with cached outputs for resume tests
	previousPipelineRun1 := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pipeline-run-1",
			Namespace: "default",
		},
		Status: v2.PipelineRunStatus{
			Steps: []*v2.PipelineRunStepInfo{
				{
					Name:        pipelinerunutils.ExecuteWorkflowStepName,
					DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
					State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
					SubSteps: []*v2.PipelineRunStepInfo{
						{
							Name:        "task1",
							DisplayName: "task1",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-1",
									},
								},
							},
						},
						{
							Name:        "task2",
							DisplayName: "task2",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-2",
									},
								},
							},
						},
						{
							Name:        "task3",
							DisplayName: "task3",
							State:       v2.PIPELINE_RUN_STEP_STATE_FAILED,
						},
					},
				},
			},
		},
	}

	// Create intermediate pipeline run for chained resume test
	previousPipelineRun2 := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pipeline-run-2",
			Namespace: "default",
		},
		Spec: v2.PipelineRunSpec{
			Resume: &v2.Resume{
				PipelineRun: &apipb.ResourceIdentifier{
					Namespace: "default",
					Name:      "test-pipeline-run-1",
				},
				ResumeFrom: []string{"task3"},
			},
		},
		Status: v2.PipelineRunStatus{
			Steps: []*v2.PipelineRunStepInfo{
				{
					Name:        pipelinerunutils.ExecuteWorkflowStepName,
					DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
					State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
					SubSteps: []*v2.PipelineRunStepInfo{
						{
							Name:        "task3",
							DisplayName: "task3",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-3",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	testCases := []struct {
		name                        string
		mockFunc                    func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient)
		pipelineRun                 *v2.PipelineRun
		expectedCondition           *apipb.Condition
		expectedExecuteWorkflowStep *v2.PipelineRunStepInfo
		expectedWorkflowRunID       string
		expectedWorkflowID          string
		errMsg                      string
	}{
		{
			name: "Condition is nil, adding step",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "",
			expectedWorkflowID:    "",
			errMsg:                "",
		},
		{
			name: "Workflow run ID is empty, starting workflow",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContet,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				blobStoreClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return([]byte(""), nil)
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecution{
					ID:    "456",
					RunID: "123",
				}, nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "123",
			expectedWorkflowID:    "456",
			errMsg:                "",
		},
		{
			name: "Workflow run ID is not empty, checking workflow status",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					WorkflowRunId: "123",
					WorkflowId:    "456",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusRunning,
				}, nil)
				// Mock the QueryWorkflow call for task progress
				workflowClient.EXPECT().QueryWorkflow(gomock.Any(), "456", "123", "task_progress", gomock.Any()).Return(nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "123",
			expectedWorkflowID:    "456",
			errMsg:                "",
		},
		{
			name: "Workflow run ID is not empty, checking workflow status -- succeeded",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					WorkflowRunId: "123",
					WorkflowId:    "456",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusCompleted,
				}, nil)
				// Mock the QueryWorkflow call for task progress
				workflowClient.EXPECT().QueryWorkflow(gomock.Any(), "456", "123", "task_progress", gomock.Any()).Return(nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				EndTime:     pbtypes.TimestampNow(),
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "123",
			expectedWorkflowID:    "456",
			errMsg:                "",
		},
		{
			name: "Pipeline run kill request - workflow is running, should cancel",
			pipelineRun: &v2.PipelineRun{
				Spec: v2.PipelineRunSpec{
					Kill: true,
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// Mock for processJobTermination
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusRunning,
				}, nil)
				workflowClient.EXPECT().CancelWorkflow(gomock.Any(), "test-workflow-id", "test-run-id", defaultengine.KillReason).Return(nil)
				// No additional mock calls needed since function returns early when terminated = true
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
				Reason: defaultengine.KillReason,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_KILLED,
				EndTime:     pbtypes.TimestampNow(),
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "test-run-id",
			expectedWorkflowID:    "test-workflow-id",
			errMsg:                "",
		},
		{
			name: "Pipeline run kill request - workflow already completed, should not cancel",
			pipelineRun: &v2.PipelineRun{
				Spec: v2.PipelineRunSpec{
					Kill: true,
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// Mock for processJobTermination - workflow already completed
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusCompleted,
				}, nil)
				// CancelWorkflow should NOT be called since workflow is already completed

				// Mock for main workflow status check
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusCompleted,
				}, nil)
				workflowClient.EXPECT().QueryWorkflow(gomock.Any(), "test-workflow-id", "test-run-id", "task_progress", gomock.Any()).Return(nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				EndTime:     pbtypes.TimestampNow(),
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "test-run-id",
			expectedWorkflowID:    "test-workflow-id",
			errMsg:                "",
		},
		{
			name: "Pipeline run kill request - error getting workflow status",
			pipelineRun: &v2.PipelineRun{
				Spec: v2.PipelineRunSpec{
					Kill: true,
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// Mock for processJobTermination - error getting status
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(nil, fmt.Errorf("workflow not found"))
				// CancelWorkflow should NOT be called due to error

				// Mock for main workflow status check
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusRunning,
				}, nil)
				workflowClient.EXPECT().QueryWorkflow(gomock.Any(), "test-workflow-id", "test-run-id", "task_progress", gomock.Any()).Return(nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "test-run-id",
			expectedWorkflowID:    "test-workflow-id",
			errMsg:                "",
		},
		{
			name: "Resume from previous pipeline run - single resume chain",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run-2",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Resume: &v2.Resume{
						PipelineRun: &apipb.ResourceIdentifier{
							Namespace: "default",
							Name:      "test-pipeline-run-1",
						},
						ResumeFrom: []string{"task2", "task3"},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContet,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				blobStoreClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return([]byte(""), nil)
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecution{
					ID:    "456",
					RunID: "123",
				}, nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "123",
			expectedWorkflowID:    "456",
			errMsg:                "",
		},
		{
			name: "Resume from previous pipeline run - chained resume",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run-3",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Resume: &v2.Resume{
						PipelineRun: &apipb.ResourceIdentifier{
							Namespace: "default",
							Name:      "test-pipeline-run-2",
						},
						ResumeFrom: []string{"task3"},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContet,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							EndTime:     pbtypes.TimestampNow(),
							StartTime:   pbtypes.TimestampNow(),
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				blobStoreClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return([]byte(""), nil)
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecution{
					ID:    "789",
					RunID: "321",
				}, nil)
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "321",
			expectedWorkflowID:    "789",
			errMsg:                "",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)
			testCase.mockFunc(workflowClient, blobStoreClient)
			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(previousPipelineRun1, previousPipelineRun2).Build()
			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)
			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandlerInstance)
			previousCondition := conditionUtils.GetCondition(pipelinerunutils.ExecuteWorkflowStepName, testCase.pipelineRun.Status.Conditions)
			condition, err := actor.Run(context.Background(), testCase.pipelineRun, previousCondition)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedCondition, condition)
				executeWorkflowStep := pipelinerunutils.GetStep(testCase.pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
				require.Equal(t, testCase.expectedExecuteWorkflowStep.State, executeWorkflowStep.State)
				require.Equal(t, testCase.expectedWorkflowID, testCase.pipelineRun.Status.WorkflowId)
				require.Equal(t, testCase.expectedWorkflowRunID, testCase.pipelineRun.Status.WorkflowRunId)
			}
		})
	}
}

func TestProcessJobTermination(t *testing.T) {
	testCases := []struct {
		name         string
		pipelineRun  *v2.PipelineRun
		mockFunc     func(workflowClient *workflowclientMock.MockWorkflowClient)
		expectError  bool
		errorMessage string
	}{
		{
			name: "Successfully cancel running workflow",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusRunning,
				}, nil)
				workflowClient.EXPECT().CancelWorkflow(gomock.Any(), "test-workflow-id", "test-run-id", defaultengine.KillReason).Return(nil)
			},
			expectError: false,
		},
		{
			name: "Do not cancel already completed workflow",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusCompleted,
				}, nil)
				// CancelWorkflow should NOT be called
			},
			expectError: false,
		},
		{
			name: "Do not cancel already terminated workflow",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusTerminated,
				}, nil)
				// CancelWorkflow should NOT be called
			},
			expectError: false,
		},
		{
			name: "Handle error when getting workflow status",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(nil, fmt.Errorf("workflow not found"))
				// CancelWorkflow should NOT be called due to error
			},
			expectError: false, // processJobTermination should not return error even if status check fails
		},
		{
			name: "Handle error when canceling workflow",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				workflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusRunning,
				}, nil)
				workflowClient.EXPECT().CancelWorkflow(gomock.Any(), "test-workflow-id", "test-run-id", defaultengine.KillReason).Return(fmt.Errorf("failed to cancel workflow"))
			},
			expectError: true, // processJobTermination should return error from CancelWorkflow
		},
		{
			name: "Skip termination when workflow ID is empty",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "",
					WorkflowRunId: "test-run-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// No calls should be made to workflow client
			},
			expectError: false,
		},
		{
			name: "Skip termination when run ID is empty",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// No calls should be made to workflow client
			},
			expectError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)

			testCase.mockFunc(workflowClient)
			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			apiHandler := apiHandler.NewFakeAPIHandler(k8sClient)

			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandler)
			err, _ = actor.processJobTermination(context.Background(), testCase.pipelineRun)

			if testCase.expectError {
				require.Error(t, err)
				if testCase.errorMessage != "" {
					require.Contains(t, err.Error(), testCase.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func setUpExecuteWorkflowActor(t *testing.T, workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient, apiHandler api.Handler) *ExecuteWorkflowActor {
	logger := zaptest.NewLogger(t)
	blobStore := blobstore.BlobStore{
		Logger: logger,
		Clients: map[string]blobstore.BlobStoreClient{
			"mock": blobStoreClient,
		},
	}
	// Create a mock config provider for testing
	configProvider, err := uberconfig.NewYAML(uberconfig.Static(map[string]interface{}{
		"workflowClient": map[string]interface{}{
			"taskList": "default",
		},
	}))
	require.NoError(t, err)

	return NewExecuteWorkflowActor(logger, workflowClient, &blobStore, apiHandler, configProvider)
}

func TestResumeFromPipelineRun(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)
	pipelineManifestContent := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
	}

	// Create previous successful pipeline runs with cached outputs
	previousPipelineRun1 := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pipeline-run-1",
			Namespace: "default",
		},
		Status: v2.PipelineRunStatus{
			Steps: []*v2.PipelineRunStepInfo{
				{
					Name:        pipelinerunutils.ExecuteWorkflowStepName,
					DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
					State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
					SubSteps: []*v2.PipelineRunStepInfo{
						{
							Name:        "task1",
							DisplayName: "task1",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-1",
									},
								},
							},
						},
						{
							Name:        "task2",
							DisplayName: "task2",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-2",
									},
								},
							},
						},
						{
							Name:        "task3",
							DisplayName: "task3",
							State:       v2.PIPELINE_RUN_STEP_STATE_FAILED,
						},
					},
				},
			},
		},
	}

	// Create intermediate pipeline run for chained resume test
	previousPipelineRun2 := &v2.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-pipeline-run-2",
			Namespace: "default",
		},
		Spec: v2.PipelineRunSpec{
			Resume: &v2.Resume{
				PipelineRun: &apipb.ResourceIdentifier{
					Namespace: "default",
					Name:      "test-pipeline-run-1",
				},
				ResumeFrom: []string{"task3"},
			},
		},
		Status: v2.PipelineRunStatus{
			Steps: []*v2.PipelineRunStepInfo{
				{
					Name:        pipelinerunutils.ExecuteWorkflowStepName,
					DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
					State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
					SubSteps: []*v2.PipelineRunStepInfo{
						{
							Name:        "task3",
							DisplayName: "task3",
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							StepCachedOutputs: &v2.PipelineRunStepCachedOutputs{
								IntermediateVars: []*apipb.ResourceIdentifier{
									{
										Namespace: "default",
										Name:      "cached-output-3",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	testCases := []struct {
		name                       string
		pipelineRun                *v2.PipelineRun
		mockSetup                  func(*testing.T, *workflowclientMock.MockWorkflowClient, *blobstoreMock.MockBlobStoreClient)
		expectedCacheEnabled       bool
		expectedCacheVersionVars   map[string]string
		expectedResumeFromDisabled []string
	}{
		{
			name: "Resume from single pipeline run",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run-2",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Resume: &v2.Resume{
						PipelineRun: &apipb.ResourceIdentifier{
							Namespace: "default",
							Name:      "test-pipeline-run-1",
						},
						ResumeFrom: []string{"task3"},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContent,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockSetup: func(t *testing.T, workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {

				blobStoreClient.EXPECT().Get(gomock.Any(), "mock://test-uniflow-tar").Return([]byte(""), nil)

				// Capture the environment variables passed to StartWorkflow
				workflowClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).DoAndReturn(func(ctx context.Context, options clientInterfaces.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
					// Extract the individual arguments from the variadic args
					tarContent := args[0].([]byte)
					starName := args[1].(string)
					workflowFuncName := args[2].(string)
					workflowArgs := args[3].([]interface{})
					kwargs := args[4].([]interface{})
					envs := args[5].(map[string]interface{})
					_ = tarContent
					_ = starName
					_ = workflowFuncName
					_ = workflowArgs
					_ = kwargs
					capturedEnvs := envs

					// Verify cache is enabled
					require.Equal(t, "true", capturedEnvs["CACHE_ENABLED"])
					require.Equal(t, "test-pipeline-run-2", capturedEnvs["CACHE_VERSION"])

					// Verify cache versions are set for successful tasks
					require.Equal(t, "test-pipeline-run-1", capturedEnvs["CACHE_VERSION_GET_task1"])
					require.Equal(t, "test-pipeline-run-1", capturedEnvs["CACHE_VERSION_GET_task2"])

					// Verify cache is disabled for resume from task
					require.Equal(t, "false", capturedEnvs["CACHE_ENABLED_task3"])

					return &clientInterfaces.WorkflowExecution{
						ID:    "456",
						RunID: "123",
					}, nil
				})
			},
			expectedCacheEnabled: true,
			expectedCacheVersionVars: map[string]string{
				"CACHE_VERSION_GET_task1": "test-pipeline-run-1",
				"CACHE_VERSION_GET_task2": "test-pipeline-run-1",
			},
			expectedResumeFromDisabled: []string{"task3"},
		},
		{
			name: "Resume from chained pipeline run",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run-3",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Resume: &v2.Resume{
						PipelineRun: &apipb.ResourceIdentifier{
							Namespace: "default",
							Name:      "test-pipeline-run-2",
						},
						ResumeFrom: []string{"task3"},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContent,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockSetup: func(t *testing.T, workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				blobStoreClient.EXPECT().Get(gomock.Any(), "mock://test-uniflow-tar").Return([]byte(""), nil)

				// Capture the environment variables passed to StartWorkflow
				workflowClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).DoAndReturn(func(ctx context.Context, options clientInterfaces.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
					// Extract the individual arguments from the variadic args
					tarContent := args[0].([]byte)
					starName := args[1].(string)
					workflowFuncName := args[2].(string)
					workflowArgs := args[3].([]interface{})
					kwargs := args[4].([]interface{})
					envs := args[5].(map[string]interface{})
					_ = tarContent
					_ = starName
					_ = workflowFuncName
					_ = workflowArgs
					_ = kwargs
					capturedEnvs := envs

					// Verify cache is enabled
					require.Equal(t, "true", capturedEnvs["CACHE_ENABLED"])
					require.Equal(t, "test-pipeline-run-3", capturedEnvs["CACHE_VERSION"])

					// Verify cache versions are set for successful tasks from the chain
					// task1 and task2 should come from test-pipeline-run-1
					require.Equal(t, "test-pipeline-run-1", capturedEnvs["CACHE_VERSION_GET_task1"])
					require.Equal(t, "test-pipeline-run-1", capturedEnvs["CACHE_VERSION_GET_task2"])
					// task3 should come from test-pipeline-run-2
					require.Equal(t, "test-pipeline-run-2", capturedEnvs["CACHE_VERSION_GET_task3"])

					// Verify cache is disabled for resume from task
					require.Equal(t, "false", capturedEnvs["CACHE_ENABLED_task3"])

					return &clientInterfaces.WorkflowExecution{
						ID:    "789",
						RunID: "321",
					}, nil
				})
			},
			expectedCacheEnabled: true,
			expectedCacheVersionVars: map[string]string{
				"CACHE_VERSION_GET_task1": "test-pipeline-run-1",
				"CACHE_VERSION_GET_task2": "test-pipeline-run-1",
				"CACHE_VERSION_GET_task3": "test-pipeline-run-2",
			},
			expectedResumeFromDisabled: []string{"task3"},
		},
		{
			name: "Resume from pipeline run - no resume spec",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run-no-resume",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					// No Resume spec
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									UniflowTar: "mock://test-uniflow-tar",
									Content:    pipelineManifestContent,
								},
							},
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ImageBuildStepName,
							DisplayName: pipelinerunutils.ImageBuildStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							Output: &pbtypes.Struct{
								Fields: map[string]*pbtypes.Value{
									pipelinerunutils.ImageBuildOutputKey: {
										Kind: &pbtypes.Value_StringValue{
											StringValue: "test-image-id",
										},
									},
								},
							},
						},
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			mockSetup: func(t *testing.T, workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				blobStoreClient.EXPECT().Get(gomock.Any(), "mock://test-uniflow-tar").Return([]byte(""), nil)

				// Capture the environment variables passed to StartWorkflow
				workflowClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).DoAndReturn(func(ctx context.Context, options clientInterfaces.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
					// Extract the individual arguments from the variadic args
					tarContent := args[0].([]byte)
					starName := args[1].(string)
					workflowFuncName := args[2].(string)
					workflowArgs := args[3].([]interface{})
					kwargs := args[4].([]interface{})
					envs := args[5].(map[string]interface{})
					_ = tarContent
					_ = starName
					_ = workflowFuncName
					_ = workflowArgs
					_ = kwargs
					capturedEnvs := envs

					// Verify cache is disabled
					require.Equal(t, "false", capturedEnvs["CACHE_ENABLED"])
					require.Equal(t, "test-pipeline-run-no-resume", capturedEnvs["CACHE_VERSION"])

					return &clientInterfaces.WorkflowExecution{
						ID:    "789",
						RunID: "321",
					}, nil
				})
			},
			expectedCacheEnabled:       false,
			expectedCacheVersionVars:   map[string]string{},
			expectedResumeFromDisabled: []string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)

			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(previousPipelineRun1, previousPipelineRun2).
				Build()

			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)

			testCase.mockSetup(t, workflowClient, blobStoreClient)

			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandlerInstance)

			// Set up the workflow step as pending with unknown condition
			previousCondition := &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			}

			condition, err := actor.Run(context.Background(), testCase.pipelineRun, previousCondition)
			require.NoError(t, err)
			require.NotNil(t, condition)
			require.Equal(t, ExecuteWorkflowType, condition.Type)
			require.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status)

			// Verify the pipeline run state was updated
			executeWorkflowStep := pipelinerunutils.GetStep(testCase.pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
			require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_RUNNING, executeWorkflowStep.State)
			require.NotEmpty(t, testCase.pipelineRun.Status.WorkflowId)
			require.NotEmpty(t, testCase.pipelineRun.Status.WorkflowRunId)
		})
	}
}
