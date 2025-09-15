package actors

import (
	"context"
	"encoding/base64"
	"testing"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	blobstoreMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowclientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestExecuteWorkflowActor(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)
	pipelineManifestContet := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
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
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&clientInterfaces.WorkflowExecution{
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
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)
			testCase.mockFunc(workflowClient, blobStoreClient)
			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient)
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

func setUpExecuteWorkflowActor(t *testing.T, workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) *ExecuteWorkflowActor {
	logger := zaptest.NewLogger(t)
	blobStore := blobstore.BlobStore{
		Logger: logger,
		Clients: map[string]blobstore.BlobStoreClient{
			"mock": blobStoreClient,
		},
	}
	return NewExecuteWorkflowActor(logger, workflowClient, &blobStore)
}
