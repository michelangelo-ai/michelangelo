package actors

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	uberconfig "go.uber.org/config"
	"go.uber.org/zap/zaptest"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	blobstoreMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowclientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestExecuteWorkflowActor(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)
	pipelineManifestContet := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
	}

	// Create a test project with worker queue annotation
	testProject := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			Annotations: map[string]string{
				"michelangelo/worker_queue": "test-task-list",
			},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
	}

	// Create a test project without worker queue annotation (for fallback testing)
	testProjectNoQueue := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:        "no-queue",
			Namespace:   "no-queue",
			Annotations: map[string]string{},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
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
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "nonexistent",
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// No mocks needed since it should fail on project fetch
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "",
			expectedWorkflowID:    "",
			errMsg:                "failed to fetch project",
		},
		{
			name: "Workflow run ID is empty, starting workflow",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
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
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, options clientInterfaces.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
						// Verify that the task list from project annotation is used
						require.Equal(t, "test-task-list", options.TaskList)
						return &clientInterfaces.WorkflowExecution{
							ID:    "456",
							RunID: "123",
						}, nil
					},
				)
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
			name: "pipeline in FAILED state, should skip all workflow operations",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_FAILED,
							StartTime:   pbtypes.TimestampNow(),
							EndTime:     pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_FALSE,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// No mock expectations
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_FAILED,
				StartTime:   pbtypes.TimestampNow(),
				EndTime:     pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "test-run-id",
			expectedWorkflowID:    "test-workflow-id",
			errMsg:                "",
		},
		{
			name: "pipeline in killed state, should skip workflow operations",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Kill: true,
				},
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        pipelinerunutils.ExecuteWorkflowStepName,
							DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
							State:       v2.PIPELINE_RUN_STEP_STATE_KILLED,
							StartTime:   pbtypes.TimestampNow(),
							EndTime:     pbtypes.TimestampNow(),
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ExecuteWorkflowType,
							Status: apipb.CONDITION_STATUS_FALSE,
							Reason: defaultengine.KillReason,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// No mock expectations
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
				StartTime:   pbtypes.TimestampNow(),
				EndTime:     pbtypes.TimestampNow(),
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
		{
			name: "Project not found - should fail",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "nonexistent",
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
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient, blobStoreClient *blobstoreMock.MockBlobStoreClient) {
				// No mocks needed since it should fail on project fetch
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedExecuteWorkflowStep: &v2.PipelineRunStepInfo{
				Name:        pipelinerunutils.ExecuteWorkflowStepName,
				DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
				State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
				StartTime:   pbtypes.TimestampNow(),
			},
			expectedWorkflowRunID: "",
			expectedWorkflowID:    "",
			errMsg:                "failed to fetch project",
		},
		{
			name: "Project without worker queue annotation - should use config fallback",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "no-queue",
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
				workflowClient.EXPECT().StartWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, options clientInterfaces.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterfaces.WorkflowExecution, error) {
						// Verify that the task list falls back to config "default"
						require.Equal(t, "default", options.TaskList)
						return &clientInterfaces.WorkflowExecution{
							ID:    "456",
							RunID: "123",
						}, nil
					},
				)
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
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(previousPipelineRun1, previousPipelineRun2, testProject, testProjectNoQueue).Build()
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

func TestGetWorkflowInputsUFStorageURL(t *testing.T) {
	testCases := []struct {
		name                 string
		pipelineRun          *v2.PipelineRun
		expectedUFStorageURL string
	}{
		{
			name: "UF_STORAGE_URL from default when no pipelineConfigMap",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil, // No manifest content
								},
							},
						},
					},
				},
			},
			expectedUFStorageURL: DefaultWorkSpaceRootURL,
		},
		{
			name: "UF_STORAGE_URL from pipelineConfigMap environ",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: createPipelineManifestWithEnviron(map[string]interface{}{
										"UF_STORAGE_URL": "s3://pipeline-config-storage",
									}),
								},
							},
						},
					},
				},
			},
			expectedUFStorageURL: "s3://pipeline-config-storage",
		},
		{
			name: "UF_STORAGE_URL preserved when environ has other vars",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: createPipelineManifestWithEnviron(map[string]interface{}{
										"CUSTOM_VAR": "custom-value",
									}),
								},
							},
						},
					},
				},
			},
			expectedUFStorageURL: DefaultWorkSpaceRootURL,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, envs, err := getWorkflowInputs(testCase.pipelineRun)

			require.NoError(t, err)
			require.NotNil(t, envs)

			// Verify UF_STORAGE_URL is set correctly
			ufStorageURL, exists := envs["UF_STORAGE_URL"]
			require.True(t, exists, "UF_STORAGE_URL should exist in environment variables")
			require.Equal(t, testCase.expectedUFStorageURL, ufStorageURL)

			// For the test case with custom vars, verify they are also present
			if testCase.name == "UF_STORAGE_URL preserved when environ has other vars" {
				customVar, exists := envs["CUSTOM_VAR"]
				require.True(t, exists, "CUSTOM_VAR should exist in environment variables")
				require.Equal(t, "custom-value", customVar)
			}
		})
	}
}

// createPipelineManifestWithEnviron creates a protobuf Any containing a manifest with environment variables
func createPipelineManifestWithEnviron(environ map[string]interface{}) *pbtypes.Any {
	// Create a manifest structure with environment variables
	manifestStruct := &pbtypes.Struct{
		Fields: map[string]*pbtypes.Value{
			"environ": {
				Kind: &pbtypes.Value_StructValue{
					StructValue: &pbtypes.Struct{
						Fields: make(map[string]*pbtypes.Value),
					},
				},
			},
		},
	}

	// Add environment variables to the environ field
	for key, value := range environ {
		manifestStruct.Fields["environ"].GetStructValue().Fields[key] = &pbtypes.Value{
			Kind: &pbtypes.Value_StringValue{
				StringValue: value.(string),
			},
		}
	}

	// Create TypedStruct and marshal it
	typedStruct := &apipb.TypedStruct{
		TypeUrl: "type.googleapis.com/michelangelo.api.v2.PipelineManifest",
		Value:   manifestStruct,
	}

	// Marshal to Any
	anyValue, _ := pbtypes.MarshalAny(typedStruct)
	return anyValue
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
			_, err = actor.processJobTermination(context.Background(), testCase.pipelineRun)

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

	// Create a test project with worker queue annotation
	testProject := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			Annotations: map[string]string{
				"michelangelo/worker_queue": "test-task-list",
			},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
	}

	// Create a test project without worker queue annotation (for fallback testing)
	testProjectNoQueue := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:        "no-queue",
			Namespace:   "no-queue",
			Annotations: map[string]string{},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
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
				WithRuntimeObjects(previousPipelineRun1, previousPipelineRun2, testProject, testProjectNoQueue).
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

func TestGetTaskList(t *testing.T) {
	// Create a test project with worker queue annotation
	testProjectWithQueue := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:      "project-with-queue",
			Namespace: "default",
			Annotations: map[string]string{
				"michelangelo/worker_queue": "custom-task-list",
			},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
	}

	// Create a test project without worker queue annotation
	testProjectNoQueue := &v2.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:        "project-no-queue",
			Namespace:   "default",
			Annotations: map[string]string{},
		},
		Spec:   v2.ProjectSpec{},
		Status: v2.ProjectStatus{},
	}

	testCases := []struct {
		name             string
		project          *v2.Project
		pipelineRun      *v2.PipelineRun
		expectedTaskList string
		expectError      bool
	}{
		{
			name:    "Project with worker queue annotation",
			project: testProjectWithQueue,
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pipeline-run",
				},
			},
			expectedTaskList: "custom-task-list",
			expectError:      false,
		},
		{
			name:    "Project without worker queue annotation - should fallback to config",
			project: testProjectNoQueue,
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pipeline-run",
				},
			},
			expectedTaskList: "default",
			expectError:      false,
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
				WithRuntimeObjects(testCase.project).
				Build()

			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)
			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandlerInstance)

			taskList, err := actor.getTaskList(testCase.project, testCase.pipelineRun)

			if testCase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedTaskList, taskList)
			}
		})
	}
}

func TestExecuteWorkflowActor_Retrieve(t *testing.T) {
	testCases := []struct {
		name              string
		pipelineRun       *v2.PipelineRun
		expectedCondition *apipb.Condition
	}{
		{
			name: "Workflow step already succeeded",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
		{
			name: "Workflow step already failed",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_FAILED,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Workflow step killed",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_KILLED,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Workflow step running with workflow IDs",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
						},
					},
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Workflow step running without workflow IDs",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Workflow not started yet",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Workflow in progress",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					WorkflowRunId: "test-run-id",
					WorkflowId:    "test-workflow-id",
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
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
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)

			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandlerInstance)
			condition, err := actor.Retrieve(context.Background(), testCase.pipelineRun, nil)

			require.NoError(t, err)
			require.Equal(t, testCase.expectedCondition, condition)
		})
	}
}

func TestConvertKwArgsMapToList(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected []interface{}
	}{
		{
			name: "Convert map to list of pairs",
			input: map[string]interface{}{
				"path":                 "glue",
				"name":                 "cola",
				"tokenizer_max_length": 128,
			},
			expected: []interface{}{
				[]interface{}{"path", "glue"},
				[]interface{}{"name", "cola"},
				[]interface{}{"tokenizer_max_length", 128},
			},
		},
		{
			name:     "Empty map",
			input:    map[string]interface{}{},
			expected: []interface{}{},
		},
		{
			name: "Already in list format",
			input: []interface{}{
				[]interface{}{"key1", "value1"},
				[]interface{}{"key2", "value2"},
			},
			expected: []interface{}{
				[]interface{}{"key1", "value1"},
				[]interface{}{"key2", "value2"},
			},
		},
		{
			name:     "Invalid format returns empty list",
			input:    "invalid",
			expected: []interface{}{},
		},
		{
			name:     "Nil returns empty list",
			input:    nil,
			expected: []interface{}{},
		},
		{
			name: "Map with various types",
			input: map[string]interface{}{
				"string_val": "test",
				"int_val":    42,
				"float_val":  3.14,
				"bool_val":   true,
			},
			expected: []interface{}{
				[]interface{}{"string_val", "test"},
				[]interface{}{"int_val", 42},
				[]interface{}{"float_val", 3.14},
				[]interface{}{"bool_val", true},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := convertKwArgsMapToList(testCase.input)
			// Since Go map iteration order is randomized, we need to compare sets
			if len(result) != len(testCase.expected) {
				t.Errorf("Expected length %d, got %d", len(testCase.expected), len(result))
				return
			}
			// Convert to map for easier comparison when input is map
			if _, isMap := testCase.input.(map[string]interface{}); isMap && len(result) > 0 {
				resultMap := make(map[string]interface{})
				for _, pair := range result {
					if pairSlice, ok := pair.([]interface{}); ok && len(pairSlice) == 2 {
						if key, ok := pairSlice[0].(string); ok {
							resultMap[key] = pairSlice[1]
						}
					}
				}
				expectedMap := make(map[string]interface{})
				for _, pair := range testCase.expected {
					if pairSlice, ok := pair.([]interface{}); ok && len(pairSlice) == 2 {
						if key, ok := pairSlice[0].(string); ok {
							expectedMap[key] = pairSlice[1]
						}
					}
				}
				require.Equal(t, expectedMap, resultMap)
			} else {
				// For non-map inputs, compare directly
				require.Equal(t, testCase.expected, result)
			}
		})
	}
}

func TestGetWorkflowInputsWithYamlBasedPipeline(t *testing.T) {
	testCases := []struct {
		name            string
		pipelineRun     *v2.PipelineRun
		expectedArgs    int // Number of args (workflow_config + task_configs)
		expectedEnvKeys []string
		expectError     bool
	}{
		{
			name: "Yaml-based pipeline with both workflow_config and task_configs",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							WorkflowConfigKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"workflow_name": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "test-workflow",
												},
											},
											"version": {
												Kind: &pbtypes.Value_NumberValue{
													NumberValue: 1.0,
												},
											},
										},
									},
								},
							},
							WorkflowTaskConfigsKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"task1": {
												Kind: &pbtypes.Value_StructValue{
													StructValue: &pbtypes.Struct{
														Fields: map[string]*pbtypes.Value{
															"task_name": {
																Kind: &pbtypes.Value_StringValue{
																	StringValue: "test-task-1",
																},
															},
														},
													},
												},
											},
											"task2": {
												Kind: &pbtypes.Value_StructValue{
													StructValue: &pbtypes.Struct{
														Fields: map[string]*pbtypes.Value{
															"task_name": {
																Kind: &pbtypes.Value_StringValue{
																	StringValue: "test-task-2",
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
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil,
								},
							},
						},
					},
				},
			},
			expectedArgs:    2, // workflow_config + task_configs
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME", "YAML_BASED_PIPELINE"},
			expectError:     false,
		},
		{
			name: "Yaml-based pipeline with only task_configs",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							WorkflowTaskConfigsKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"task1": {
												Kind: &pbtypes.Value_StructValue{
													StructValue: &pbtypes.Struct{
														Fields: map[string]*pbtypes.Value{
															"enabled": {
																Kind: &pbtypes.Value_BoolValue{
																	BoolValue: true,
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
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil,
								},
							},
						},
					},
				},
			},
			expectedArgs:    1, // Only task_configs
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME", "YAML_BASED_PIPELINE"},
			expectError:     false,
		},
		{
			name: "Yaml-based pipeline with only workflow_config",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							WorkflowConfigKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"learning_rate": {
												Kind: &pbtypes.Value_NumberValue{
													NumberValue: 0.001,
												},
											},
										},
									},
								},
							},
							// Must have task_configs to be detected as Yaml-based
							// This test case should NOT set YAML_BASED_PIPELINE
						},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil,
								},
							},
						},
					},
				},
			},
			expectedArgs:    0, // workflow_config alone doesn't trigger Yaml-based detection
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME"},
			expectError:     false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			args, kwArgs, envs, err := getWorkflowInputs(testCase.pipelineRun)

			if testCase.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify args count
			require.Len(t, args, testCase.expectedArgs)

			// Verify kwArgs is empty for Yaml-based pipelines
			require.Empty(t, kwArgs)

			// Verify environment variables contain expected keys
			for _, key := range testCase.expectedEnvKeys {
				_, exists := envs[key]
				require.True(t, exists, "Environment variable %s should exist", key)
			}

			// Verify YAML_BASED_PIPELINE flag is set correctly
			if testCase.expectedArgs > 0 {
				yamlFlag, exists := envs["YAML_BASED_PIPELINE"]
				require.True(t, exists, "YAML_BASED_PIPELINE should be set for yaml-based pipelines")
				require.Equal(t, "true", yamlFlag)
			}
		})
	}
}

func TestGetWorkflowInputsWithPythonSDKBasedPipeline(t *testing.T) {
	testCases := []struct {
		name            string
		pipelineRun     *v2.PipelineRun
		expectedKwArgs  int
		expectedEnvKeys []string
		expectError     bool
	}{
		{
			name: "Uniflow pipeline with kw_args from Spec.Input",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							WorkflowKWArgsKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"path": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "glue",
												},
											},
											"name": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "cola",
												},
											},
											"tokenizer_max_length": {
												Kind: &pbtypes.Value_NumberValue{
													NumberValue: 128,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil, // No manifest content
								},
							},
						},
					},
				},
			},
			expectedKwArgs:  3, // path, name, tokenizer_max_length
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME"},
			expectError:     false,
		},
		{
			name: "Uniflow pipeline with only kw_args (no args)",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							WorkflowKWArgsKey: {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"dataset": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "mnist",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil,
								},
							},
						},
					},
				},
			},
			expectedKwArgs:  1, // dataset
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME"},
			expectError:     false,
		},
		{
			name: "Uniflow pipeline with environ",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							"environ": {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"CUSTOM_VAR": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "custom_value",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							Spec: v2.PipelineSpec{
								Manifest: &v2.PipelineManifest{
									Content: nil,
								},
							},
						},
					},
				},
			},
			expectedKwArgs:  0,
			expectedEnvKeys: []string{"UF_STORAGE_URL", "MA_NAMESPACE", "MA_PIPELINE_RUN_NAME", "CUSTOM_VAR"},
			expectError:     false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			args, kwArgs, envs, err := getWorkflowInputs(testCase.pipelineRun)

			if testCase.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify args is empty (args support will be added in a separate diff)
			require.Empty(t, args)

			// Verify kw_args count (since order is not guaranteed)
			require.Len(t, kwArgs, testCase.expectedKwArgs)

			// Verify that each kw_arg is a [key, value] pair
			for _, kwArg := range kwArgs {
				pair, ok := kwArg.([]interface{})
				require.True(t, ok, "Each kw_arg should be a list")
				require.Len(t, pair, 2, "Each kw_arg pair should have exactly 2 elements")
			}

			// Verify environment variables contain expected keys
			for _, key := range testCase.expectedEnvKeys {
				_, exists := envs[key]
				require.True(t, exists, "Environment variable %s should exist", key)
			}
		})
	}
}

func TestProcessManualRetrySpec(t *testing.T) {
	testCases := []struct {
		name            string
		pipelineRun     *v2.PipelineRun
		mockFunc        func(workflowClient *workflowclientMock.MockWorkflowClient)
		expectedError   string
		expectedRunning bool
	}{
		{
			name: "No retry info - should not process",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					RetryInfo: nil,
				},
				Status: v2.PipelineRunStatus{
					WorkflowRunId: "current-run-id",
				},
			},
			mockFunc:        func(workflowClient *workflowclientMock.MockWorkflowClient) {},
			expectedError:   "",
			expectedRunning: false,
		},
		{
			name: "Empty activity ID - should not process",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					RetryInfo: &v2.RetryInfo{
						ActivityId: "",
					},
				},
				Status: v2.PipelineRunStatus{
					WorkflowRunId: "current-run-id",
				},
			},
			mockFunc:        func(workflowClient *workflowclientMock.MockWorkflowClient) {},
			expectedError:   "",
			expectedRunning: false,
		},
		{
			name: "WorkflowRunId matches current - should process retry",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					RetryInfo: &v2.RetryInfo{
						ActivityId:    "test-activity-1",
						WorkflowId:    "test-workflow-id",
						WorkflowRunId: "current-run-id",
						Reason:        "Test retry",
					},
				},
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "current-run-id",
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ExecuteWorkflowStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_FAILED,
						},
					},
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				// Mock GetWorkflowExecutionHistory
				mockHistory := &clientInterfaces.WorkflowHistory{
					Events: []clientInterfaces.HistoryEvent{
						{
							EventID:   1,
							EventType: "ActivityTaskCompleted",
						},
						{
							EventID:   2,
							EventType: "ActivityTaskScheduled",
							Details: map[string]interface{}{
								"activity_id": "test-activity-1",
							},
						},
					},
				}
				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"current-run-id",
					gomock.Any(),
					int32(5000),
				).Return(mockHistory, nil)

				// Mock ResetWorkflow
				workflowClient.EXPECT().ResetWorkflow(
					gomock.Any(),
					gomock.Any(),
				).Return(&clientInterfaces.WorkflowExecution{
					ID:    "test-workflow-id",
					RunID: "new-run-id",
				}, nil)
			},
			expectedError:   "",
			expectedRunning: true,
		},
		{
			name: "WorkflowRunId differs - should not process",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					RetryInfo: &v2.RetryInfo{
						ActivityId:    "test-activity-1",
						WorkflowRunId: "old-run-id",
					},
				},
				Status: v2.PipelineRunStatus{
					WorkflowRunId: "new-run-id", // Different from retry
				},
			},
			mockFunc:        func(workflowClient *workflowclientMock.MockWorkflowClient) {},
			expectedError:   "",
			expectedRunning: false,
		},
		{
			name: "Reset workflow fails",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "default",
				},
				Spec: v2.PipelineRunSpec{
					RetryInfo: &v2.RetryInfo{
						ActivityId:    "test-activity-1",
						WorkflowId:    "test-workflow-id",
						WorkflowRunId: "current-run-id", // Same as current to trigger processing
						Reason:        "Manual retry",
					},
				},
				Status: v2.PipelineRunStatus{
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "current-run-id", // Same as retry to trigger processing
				},
			},
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				// Mock GetWorkflowExecutionHistory
				mockHistory := &clientInterfaces.WorkflowHistory{
					Events: []clientInterfaces.HistoryEvent{
						{
							EventID:   1,
							EventType: "ActivityTaskCompleted",
						},
						{
							EventID:   2,
							EventType: "ActivityTaskScheduled",
							Details: map[string]interface{}{
								"activity_id": "test-activity-1",
							},
						},
					},
				}
				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"current-run-id",
					gomock.Any(),
					int32(5000),
				).Return(mockHistory, nil)

				// Mock ResetWorkflow failure
				workflowClient.EXPECT().ResetWorkflow(
					gomock.Any(),
					gomock.Any(),
				).Return(nil, fmt.Errorf("reset failed"))
			},
			expectedError:   "workflow reset failed for activity test-activity-1: reset failed",
			expectedRunning: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockWorkflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			mockBlobStore := blobstoreMock.NewMockBlobStoreClient(ctrl)

			testCase.mockFunc(mockWorkflowClient)

			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)

			actor := setUpExecuteWorkflowActor(t, mockWorkflowClient, mockBlobStore, apiHandlerInstance)

			retryErr := actor.processManualRetrySpec(context.Background(), testCase.pipelineRun)

			if testCase.expectedError != "" {
				require.Error(t, retryErr)
				require.Contains(t, retryErr.Error(), testCase.expectedError)
			} else {
				require.NoError(t, retryErr)
			}

			if testCase.expectedRunning {
				require.Equal(t, v2.PIPELINE_RUN_STATE_RUNNING, testCase.pipelineRun.Status.State)
				executeStep := pipelinerunutils.GetStep(testCase.pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
				if executeStep != nil {
					require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_RUNNING, executeStep.State)
				}
			}
		})
	}
}

func TestFindTaskResetEventIDByActivityID(t *testing.T) {
	testCases := []struct {
		name            string
		workflowID      string
		runID           string
		firstActivityID string
		mockFunc        func(workflowClient *workflowclientMock.MockWorkflowClient)
		expectedEventID int64
		expectedError   string
	}{
		{
			name:            "Find reset event successfully",
			workflowID:      "test-workflow-id",
			runID:           "test-run-id",
			firstActivityID: "test-activity-1",
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				mockHistory := &clientInterfaces.WorkflowHistory{
					Events: []clientInterfaces.HistoryEvent{
						{
							EventID:   1,
							EventType: "WorkflowExecutionStarted",
						},
						{
							EventID:   2,
							EventType: "DecisionTaskCompleted",
						},
						{
							EventID:   3,
							EventType: "ActivityTaskScheduled",
							Details: map[string]interface{}{
								"activity_id": "test-activity-1",
							},
						},
						{
							EventID:   4,
							EventType: "ActivityTaskFailed",
						},
					},
				}
				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
					gomock.Any(),
					int32(5000),
				).Return(mockHistory, nil)
			},
			expectedEventID: 2,
			expectedError:   "",
		},
		{
			name:            "Activity not found",
			workflowID:      "test-workflow-id",
			runID:           "test-run-id",
			firstActivityID: "missing-activity",
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				mockHistory := &clientInterfaces.WorkflowHistory{
					Events: []clientInterfaces.HistoryEvent{
						{
							EventID:   1,
							EventType: "WorkflowExecutionStarted",
						},
						{
							EventID:   2,
							EventType: "ActivityTaskScheduled",
							Details: map[string]interface{}{
								"activity_id": "different-activity",
							},
						},
					},
				}
				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
					gomock.Any(),
					int32(5000),
				).Return(mockHistory, nil)
			},
			expectedEventID: 0,
			expectedError:   "could not find scheduled event for first activity missing-activity",
		},
		{
			name:            "History retrieval fails",
			workflowID:      "test-workflow-id",
			runID:           "test-run-id",
			firstActivityID: "test-activity-1",
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods (even though they won't be called due to error)
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
					gomock.Any(),
					int32(5000),
				).Return(nil, fmt.Errorf("history error"))
			},
			expectedEventID: 0,
			expectedError:   "failed to get workflow history: history error",
		},
		{
			name:            "No safe reset boundary found",
			workflowID:      "test-workflow-id",
			runID:           "test-run-id",
			firstActivityID: "test-activity-1",
			mockFunc: func(workflowClient *workflowclientMock.MockWorkflowClient) {
				// Mock event type interface methods
				workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
				workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
				workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()

				mockHistory := &clientInterfaces.WorkflowHistory{
					Events: []clientInterfaces.HistoryEvent{
						{
							EventID:   1,
							EventType: "ActivityTaskScheduled",
							Details: map[string]interface{}{
								"activity_id": "test-activity-1",
							},
						},
					},
				}
				workflowClient.EXPECT().GetWorkflowExecutionHistory(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
					gomock.Any(),
					int32(5000),
				).Return(mockHistory, nil)
			},
			expectedEventID: 0,
			expectedError:   "could not find safe reset boundary before first activity test-activity-1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockWorkflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			mockBlobStore := blobstoreMock.NewMockBlobStoreClient(ctrl)

			testCase.mockFunc(mockWorkflowClient)

			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)

			actor := setUpExecuteWorkflowActor(t, mockWorkflowClient, mockBlobStore, apiHandlerInstance)

			eventID, err := actor.findTaskResetEventIDByActivityID(
				context.Background(),
				testCase.workflowID,
				testCase.runID,
				testCase.firstActivityID,
			)

			if testCase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.expectedError)
				require.Equal(t, int64(0), eventID)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedEventID, eventID)
			}
		})
	}
}

func TestGetWorkflowUrl(t *testing.T) {
	testCases := []struct {
		name        string
		configSetup map[string]interface{}
		inputName   string
		expectedUrl string
		expectError bool
	}{
		{
			name: "Valid config with workflow URL template",
			configSetup: map[string]interface{}{
				"workflowClient": map[string]interface{}{
					"taskList":           "default",
					"executionUrlFormat": "http://cadence-web:8080/domain/{{.Domain}}/workflows/{{.ExecutionID}}",
					"domain":             "michelangelo",
				},
			},
			inputName:   "test-pipeline-run",
			expectedUrl: "http://cadence-web:8080/domain/michelangelo/workflows/test-pipeline-run",
			expectError: false,
		},
		{
			name: "Config with different domain",
			configSetup: map[string]interface{}{
				"workflowClient": map[string]interface{}{
					"taskList":           "custom-queue",
					"executionUrlFormat": "https://temporal-ui.prod:7243/namespaces/{{.Domain}}/workflows/{{.ExecutionID}}",
					"domain":             "production",
				},
			},
			inputName:   "prod-pipeline-execution",
			expectedUrl: "https://temporal-ui.prod:7243/namespaces/production/workflows/prod-pipeline-execution",
			expectError: false,
		},
		{
			name: "Missing workflow config - should return empty string",
			configSetup: map[string]interface{}{
				"workflowClient": map[string]interface{}{
					"taskList": "default",
					// Missing workflowUrl and domain
				},
			},
			inputName:   "test-pipeline",
			expectedUrl: "",
			expectError: false, // Method returns empty string on config error
		},
		{
			name: "Empty pipeline name",
			configSetup: map[string]interface{}{
				"workflowClient": map[string]interface{}{
					"taskList":           "default",
					"executionUrlFormat": "http://localhost:8080/domain/{{.Domain}}/workflows/{{.ExecutionID}}",
					"domain":             "default",
				},
			},
			inputName:   "",
			expectedUrl: "http://localhost:8080/domain/default/workflows/",
			expectError: false,
		},
		{
			name: "Complex pipeline name with special characters",
			configSetup: map[string]interface{}{
				"workflowClient": map[string]interface{}{
					"taskList":           "default",
					"executionUrlFormat": "http://cadence-web:8080/domain/{{.Domain}}/workflows/{{.ExecutionID}}",
					"domain":             "test-domain",
				},
			},
			inputName:   "my-pipeline-run-123_test",
			expectedUrl: "http://cadence-web:8080/domain/test-domain/workflows/my-pipeline-run-123_test",
			expectError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)

			// Create config provider with test case specific config
			configProvider, err := uberconfig.NewYAML(uberconfig.Static(testCase.configSetup))
			require.NoError(t, err)

			// Create actor with test-specific config
			logger := zaptest.NewLogger(t)
			blobStore := blobstore.BlobStore{
				Logger: logger,
				Clients: map[string]blobstore.BlobStoreClient{
					"mock": blobStoreClient,
				},
			}

			scheme := runtime.NewScheme()
			err = v2.AddToScheme(scheme)
			require.NoError(t, err)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			apiHandlerInstance := apiHandler.NewFakeAPIHandler(k8sClient)

			actor := NewExecuteWorkflowActor(logger, workflowClient, &blobStore, apiHandlerInstance, configProvider)

			// Test the GetWorkflowUrl method
			result := actor.GetWorkflowUrl(testCase.inputName)

			if testCase.expectError {
				require.Empty(t, result)
			} else {
				require.Equal(t, testCase.expectedUrl, result)
			}
		})
	}
}

func TestGetStepInfoFromTaskProgressInput(t *testing.T) {
	testCases := []struct {
		name          string
		taskProgress  TaskProgress
		expectedInput *pbtypes.Struct
	}{
		{
			name: "input with args and kwargs",
			taskProgress: TaskProgress{
				TaskName:  "train",
				TaskPath:  "examples.train",
				TaskState: "succeeded",
				Input:     `{"args": ["a", "b"], "kwargs": {"lr": 0.01}}`,
			},
			expectedInput: &pbtypes.Struct{
				Fields: map[string]*pbtypes.Value{
					"args": {Kind: &pbtypes.Value_ListValue{ListValue: &pbtypes.ListValue{
						Values: []*pbtypes.Value{
							{Kind: &pbtypes.Value_StringValue{StringValue: "a"}},
							{Kind: &pbtypes.Value_StringValue{StringValue: "b"}},
						},
					}}},
					"kwargs": {Kind: &pbtypes.Value_StructValue{StructValue: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							"lr": {Kind: &pbtypes.Value_NumberValue{NumberValue: 0.01}},
						},
					}}},
				},
			},
		},
		{
			name: "empty input",
			taskProgress: TaskProgress{
				TaskName:  "train",
				TaskPath:  "examples.train",
				TaskState: "running",
				Input:     "",
			},
			expectedInput: nil,
		},
		{
			name: "invalid json input",
			taskProgress: TaskProgress{
				TaskName:  "train",
				TaskPath:  "examples.train",
				TaskState: "running",
				Input:     "not-json",
			},
			expectedInput: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stepInfo := getStepInfoFromTaskProgress(&tc.taskProgress, "test-ns")
			require.Equal(t, tc.expectedInput, stepInfo.Input)
		})
	}
}

func TestEnrichStepOutput(t *testing.T) {
	testCases := []struct {
		name           string
		taskProgress   TaskProgress
		cachedOutput   *v2.CachedOutput
		blobContent    string
		expectOutput   bool
		expectedFields []string
	}{
		{
			name: "variable type with JSON object output",
			taskProgress: TaskProgress{
				Output: "uf-vars-abc",
			},
			cachedOutput: &v2.CachedOutput{
				Spec: v2.CachedOutputSpec{
					Type:       v2.CACHED_OUTPUT_TYPE_VARIABLE,
					StorageUri: "mock://result.json",
				},
			},
			blobContent:    `{"accuracy": 0.95, "loss": 0.05}`,
			expectOutput:   true,
			expectedFields: []string{"accuracy", "loss"},
		},
		{
			name: "variable type with JSON array output wrapped in result",
			taskProgress: TaskProgress{
				Output: "uf-vars-arr",
			},
			cachedOutput: &v2.CachedOutput{
				Spec: v2.CachedOutputSpec{
					Type:       v2.CACHED_OUTPUT_TYPE_VARIABLE,
					StorageUri: "mock://result.json",
				},
			},
			blobContent:    `[1, 2, 3]`,
			expectOutput:   true,
			expectedFields: []string{"result"},
		},
		{
			name: "checkpoint type is skipped",
			taskProgress: TaskProgress{
				Output: "uf-ckpt-abc",
			},
			cachedOutput: &v2.CachedOutput{
				Spec: v2.CachedOutputSpec{
					Type:       v2.CACHED_OUTPUT_TYPE_TRAINING_CKPT,
					StorageUri: "mock://ckpt",
				},
			},
			expectOutput: false,
		},
		{
			name: "empty output name is skipped",
			taskProgress: TaskProgress{
				Output: "",
			},
			expectOutput: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			workflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
			blobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctrl)

			scheme := runtime.NewScheme()
			require.NoError(t, v2.AddToScheme(scheme))
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			if tc.cachedOutput != nil {
				tc.cachedOutput.Name = tc.taskProgress.Output
				tc.cachedOutput.Namespace = "test-ns"
				require.NoError(t, fakeClient.Create(context.Background(), tc.cachedOutput))
			}
			if tc.blobContent != "" {
				blobStoreClient.EXPECT().Get(gomock.Any(), "mock://result.json").Return([]byte(tc.blobContent), nil)
			}

			apiHandlerInstance := apiHandler.NewFakeAPIHandler(fakeClient)
			actor := setUpExecuteWorkflowActor(t, workflowClient, blobStoreClient, apiHandlerInstance)

			stepInfo := &v2.PipelineRunStepInfo{}
			actor.enrichStepOutput(context.Background(), "test-ns", &tc.taskProgress, stepInfo)

			if !tc.expectOutput {
				require.Nil(t, stepInfo.Output)
			} else {
				require.NotNil(t, stepInfo.Output)
				for _, field := range tc.expectedFields {
					_, ok := stepInfo.Output.Fields[field]
					require.True(t, ok, "expected field %q in output", field)
				}
			}
		})
	}
}

// TestExecuteWorkflowStepRetryHistory tests that processManualRetrySpec preserves
// the terminal step state in AttemptDetails before mutating the step to RUNNING.
func TestExecuteWorkflowStepRetryHistory(t *testing.T) {
	mockEventTypes := func(workflowClient *workflowclientMock.MockWorkflowClient) {
		workflowClient.EXPECT().GetActivityTaskScheduledEventType().Return("ActivityTaskScheduled").AnyTimes()
		workflowClient.EXPECT().GetActivityTaskCompletedEventType().Return("ActivityTaskCompleted").AnyTimes()
		workflowClient.EXPECT().GetDecisionTaskCompletedEventType().Return("DecisionTaskCompleted").AnyTimes()
	}

	mockHistoryForRunID := func(workflowClient *workflowclientMock.MockWorkflowClient, runID string) {
		workflowClient.EXPECT().GetWorkflowExecutionHistory(
			gomock.Any(), "test-workflow-id", runID, gomock.Any(), int32(5000),
		).Return(&clientInterfaces.WorkflowHistory{
			Events: []clientInterfaces.HistoryEvent{
				{EventID: 1, EventType: "ActivityTaskCompleted"},
				{EventID: 2, EventType: "ActivityTaskScheduled", Details: map[string]interface{}{"activity_id": "test-activity-1"}},
			},
		}, nil)
	}

	mockReset := func(workflowClient *workflowclientMock.MockWorkflowClient, newRunID string) {
		workflowClient.EXPECT().ResetWorkflow(gomock.Any(), gomock.Any()).Return(
			&clientInterfaces.WorkflowExecution{ID: "test-workflow-id", RunID: newRunID}, nil,
		)
	}

	buildPipelineRun := func() *v2.PipelineRun {
		return &v2.PipelineRun{
			ObjectMeta: v1.ObjectMeta{Name: "test-pipeline-run", Namespace: "default"},
			Spec: v2.PipelineRunSpec{
				RetryInfo: &v2.RetryInfo{
					ActivityId:    "test-activity-1",
					WorkflowId:    "test-workflow-id",
					WorkflowRunId: "current-run-id",
					Reason:        "Test retry",
				},
			},
			Status: v2.PipelineRunStatus{
				WorkflowId:    "test-workflow-id",
				WorkflowRunId: "current-run-id",
				Steps: []*v2.PipelineRunStepInfo{
					{
						Name:       pipelinerunutils.ExecuteWorkflowStepName,
						State:      v2.PIPELINE_RUN_STEP_STATE_FAILED,
						Message:    "activity failed: timeout",
						ActivityId: "workflow-activity-123",
						LogUrl:     "s3://logs/workflow-failed/",
						StartTime:  &pbtypes.Timestamp{Seconds: 1000},
						EndTime:    &pbtypes.Timestamp{Seconds: 2000},
						SubSteps: []*v2.PipelineRunStepInfo{
							{Name: "task1", State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, LogUrl: "s3://logs/task1/"},
							{Name: "task2", State: v2.PIPELINE_RUN_STEP_STATE_FAILED, LogUrl: "s3://logs/task2/"},
						},
					},
				},
			},
		}
	}

	t.Run("first retry preserves failed state in AttemptDetails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockWorkflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
		mockBlobStore := blobstoreMock.NewMockBlobStoreClient(ctrl)
		mockEventTypes(mockWorkflowClient)
		mockHistoryForRunID(mockWorkflowClient, "current-run-id")
		mockReset(mockWorkflowClient, "new-run-id")

		scheme := runtime.NewScheme()
		require.NoError(t, v2.AddToScheme(scheme))
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		actor := setUpExecuteWorkflowActor(t, mockWorkflowClient, mockBlobStore, apiHandler.NewFakeAPIHandler(k8sClient))

		pipelineRun := buildPipelineRun()
		require.NoError(t, actor.processManualRetrySpec(context.Background(), pipelineRun))

		executeStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
		require.NotNil(t, executeStep)

		// The step itself should now be RUNNING
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_RUNNING, executeStep.State)

		// Should have exactly one attempt detail entry
		require.Len(t, executeStep.AttemptDetails, 1)
		attempt := executeStep.AttemptDetails[0]

		// Attempt ID should be "1"
		require.Equal(t, "1", attempt.AttemptId)

		// Snapshot should capture the pre-retry FAILED state, not RUNNING
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, attempt.StepInfo.State)
		require.Equal(t, "activity failed: timeout", attempt.StepInfo.Message)
		require.Equal(t, "s3://logs/workflow-failed/", attempt.StepInfo.LogUrl)

		// Snapshot should preserve sub-steps
		require.Len(t, attempt.StepInfo.SubSteps, 2)
		require.Equal(t, "task1", attempt.StepInfo.SubSteps[0].Name)
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED, attempt.StepInfo.SubSteps[0].State)
		require.Equal(t, "task2", attempt.StepInfo.SubSteps[1].Name)
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, attempt.StepInfo.SubSteps[1].State)

		// Nested AttemptDetails on the snapshot must be nil (flat structure guarantee)
		require.Nil(t, attempt.StepInfo.AttemptDetails)
	})

	t.Run("second retry produces two entries with correct attempt IDs", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockWorkflowClient := workflowclientMock.NewMockWorkflowClient(ctrl)
		mockBlobStore := blobstoreMock.NewMockBlobStoreClient(ctrl)
		mockEventTypes(mockWorkflowClient)
		// Mocks for first retry
		mockHistoryForRunID(mockWorkflowClient, "current-run-id")
		mockReset(mockWorkflowClient, "new-run-id")

		scheme := runtime.NewScheme()
		require.NoError(t, v2.AddToScheme(scheme))
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		actor := setUpExecuteWorkflowActor(t, mockWorkflowClient, mockBlobStore, apiHandler.NewFakeAPIHandler(k8sClient))

		pipelineRun := buildPipelineRun()

		// First retry
		require.NoError(t, actor.processManualRetrySpec(context.Background(), pipelineRun))

		// Simulate the step failing again after the first retry
		executeStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
		executeStep.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		executeStep.Message = "activity failed: OOM"
		// Reset RetryInfo to trigger a second retry
		pipelineRun.Status.WorkflowRunId = "new-run-id"
		pipelineRun.Spec.RetryInfo = &v2.RetryInfo{
			ActivityId:    "test-activity-1",
			WorkflowId:    "test-workflow-id",
			WorkflowRunId: "new-run-id",
			Reason:        "Second retry",
		}

		// Mocks for second retry
		mockHistoryForRunID(mockWorkflowClient, "new-run-id")
		mockReset(mockWorkflowClient, "newer-run-id")

		// Second retry
		require.NoError(t, actor.processManualRetrySpec(context.Background(), pipelineRun))

		executeStep = pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
		require.Len(t, executeStep.AttemptDetails, 2)

		// First attempt snapshot
		require.Equal(t, "1", executeStep.AttemptDetails[0].AttemptId)
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, executeStep.AttemptDetails[0].StepInfo.State)
		require.Equal(t, "activity failed: timeout", executeStep.AttemptDetails[0].StepInfo.Message)
		require.Nil(t, executeStep.AttemptDetails[0].StepInfo.AttemptDetails)

		// Second attempt snapshot
		require.Equal(t, "2", executeStep.AttemptDetails[1].AttemptId)
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_FAILED, executeStep.AttemptDetails[1].StepInfo.State)
		require.Equal(t, "activity failed: OOM", executeStep.AttemptDetails[1].StepInfo.Message)
		require.Nil(t, executeStep.AttemptDetails[1].StepInfo.AttemptDetails)

		// The live step should be RUNNING
		require.Equal(t, v2.PIPELINE_RUN_STEP_STATE_RUNNING, executeStep.State)
	})
}
