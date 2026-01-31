package pipelinerun

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"

	"github.com/stretchr/testify/require"
	uberconfig "go.uber.org/config"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	blobStorageClientMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowClientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestReconcile(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)

	pipelineManifestContent := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
	}
	testCases := []struct {
		name                      string
		initialObjects            []client.Object
		mockFunc                  func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient)
		expectedConditions        []*apipb.Condition
		expectedPipelineRunStatus v2.PipelineRunStatus
		expectedSteps             []*v2.PipelineRunStepInfo
		errMsg                    string
		expectedResult            ctrl.Result
	}{
		{
			name: "first reconcile, SourcePipeline actor loads pipeline into status",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// No mocks needed for first reconcile:
				// - SourcePipeline.Retrieve() returns FALSE (pipeline not loaded yet)
				// - SourcePipeline.Run() is called, loads pipeline from k8s API (provided in initialObjects)
				// - ImageBuild.Retrieve() returns FALSE, but Run() not called (only first non-satisfied actor runs)
				// - ExecuteWorkflow.Retrieve() returns FALSE, but Run() not called
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:    actors.ImageBuildType,
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "Missing image ID",
					Message: "Source pipeline is available but missing michelangelo/uniflow-image annotation",
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State: v2.PIPELINE_RUN_STATE_RUNNING,
				SourcePipeline: &v2.SourcePipeline{
					Pipeline: &v2.Pipeline{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
				},
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
			},
		},
		{
			name: "second reconcile, ImageBuild actor runs but fails due to missing image annotation",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
					Status: v2.PipelineRunStatus{
						Conditions: []*apipb.Condition{
							{
								Type:   actors.SourcePipelineType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_FALSE,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_FALSE,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.SourcePipelineStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
						},
						SourcePipeline: &v2.SourcePipeline{
							Pipeline: &v2.Pipeline{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-pipeline",
									Namespace: "test-namespace",
									// No image ID annotation, this will fail the image build step
								},
							},
						},
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
						// No image ID annotation, this will fail the image build step
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// SourcePipeline.Retrieve() returns TRUE (already loaded)
				// ImageBuild.Retrieve() returns FALSE (missing annotation)
				// ImageBuild.Run() is called, returns FALSE with error reason
				// ExecuteWorkflow.Retrieve() returns FALSE, but Run() not called (single actor per cycle)
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State: v2.PIPELINE_RUN_STATE_FAILED,
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_FAILED,
				},
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
		},
		{
			name: "third reconcile, ExecuteWorkflow actor starts workflow",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
					Status: v2.PipelineRunStatus{
						Conditions: []*apipb.Condition{
							{
								Type:   actors.SourcePipelineType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_FALSE,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.SourcePipelineStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
						},
						SourcePipeline: &v2.SourcePipeline{
							Pipeline: &v2.Pipeline{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-pipeline",
									Namespace: "test-namespace",
									Annotations: map[string]string{
										pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
									},
								},
								Spec: v2.PipelineSpec{
									Manifest: &v2.PipelineManifest{
										Content:    pipelineManifestContent,
										UniflowTar: "mock://test-uniflow-tar",
									},
								},
							},
						},
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
						},
					},
					Spec: v2.PipelineSpec{
						Manifest: &v2.PipelineManifest{
							Content:    pipelineManifestContent,
							UniflowTar: "mock://test-uniflow-tar",
						},
					},
				},
				&v2.Project{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-namespace",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							"michelangelo/worker_queue": "test-task-list",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// SourcePipeline.Retrieve() returns TRUE
				// ImageBuild.Retrieve() returns TRUE
				// ExecuteWorkflow.Retrieve() returns FALSE (workflow not started)
				// ExecuteWorkflow.Run() is called - starts workflow
				mockBlobStorageClient.EXPECT().Get(gomock.Any(), "mock://test-uniflow-tar").Return([]byte("mock-tar-content"), nil)
				mockWorkflowClient.EXPECT().StartWorkflow(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).Return(&clientInterfaces.WorkflowExecution{
					ID:    "test-workflow-id",
					RunID: "test-run-id",
				}, nil)
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State:         v2.PIPELINE_RUN_STATE_RUNNING,
				WorkflowId:    "test-workflow-id",
				WorkflowRunId: "test-run-id",
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
				},
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
			},
		},
		{
			name: "fourth reconcile, workflow completes and returns TRUE, triggers requeue",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
					Status: v2.PipelineRunStatus{
						WorkflowId:    "test-workflow-id",
						WorkflowRunId: "test-run-id",
						Conditions: []*apipb.Condition{
							{
								Type:   actors.SourcePipelineType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_UNKNOWN,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.SourcePipelineStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							},
						},
						SourcePipeline: &v2.SourcePipeline{
							Pipeline: &v2.Pipeline{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-pipeline",
									Namespace: "test-namespace",
									Annotations: map[string]string{
										pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
									},
								},
								Spec: v2.PipelineSpec{
									Manifest: &v2.PipelineManifest{
										Content:    pipelineManifestContent,
										UniflowTar: "mock://test-uniflow-tar",
									},
								},
							},
						},
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
						},
					},
					Spec: v2.PipelineSpec{
						Manifest: &v2.PipelineManifest{
							Content:    pipelineManifestContent,
							UniflowTar: "mock://test-uniflow-tar",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// SourcePipeline.Retrieve() returns TRUE
				// ImageBuild.Retrieve() returns TRUE
				// ExecuteWorkflow.Retrieve() queries workflow and sees it's completed
				mockWorkflowClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
				).Return(&clientInterfaces.WorkflowExecutionInfo{
					Status: clientInterfaces.WorkflowExecutionStatusCompleted,
				}, nil)
				// After getting execution info, it queries for task progress
				mockWorkflowClient.EXPECT().QueryWorkflow(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
					gomock.Any(),
					gomock.Any(),
				).Return(nil)
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State:         v2.PIPELINE_RUN_STATE_RUNNING, // Still RUNNING because criticalCondition (returned from defaultEngine) is still non-terminal
				WorkflowId:    "test-workflow-id",
				WorkflowRunId: "test-run-id",
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      true, // Requeues because criticalCondition (returned from defaultEngine) is still non-terminal
				RequeueAfter: 10 * time.Second,
			},
		},
		{
			name: "fifth reconcile, all conditions TRUE from Retrieve, terminal success",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
					Status: v2.PipelineRunStatus{
						WorkflowId:    "test-workflow-id",
						WorkflowRunId: "test-run-id",
						Conditions: []*apipb.Condition{
							{
								Type:   actors.SourcePipelineType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.SourcePipelineStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
						},
						SourcePipeline: &v2.SourcePipeline{
							Pipeline: &v2.Pipeline{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-pipeline",
									Namespace: "test-namespace",
									Annotations: map[string]string{
										pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
									},
								},
								Spec: v2.PipelineSpec{
									Manifest: &v2.PipelineManifest{
										Content:    pipelineManifestContent,
										UniflowTar: "mock://test-uniflow-tar",
									},
								},
							},
						},
						State: v2.PIPELINE_RUN_STATE_RUNNING,
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
						},
					},
					Spec: v2.PipelineSpec{
						Manifest: &v2.PipelineManifest{
							Content:    pipelineManifestContent,
							UniflowTar: "mock://test-uniflow-tar",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// All Retrieve() calls return TRUE
				// No Run() is called
				// No mocks needed
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State:         v2.PIPELINE_RUN_STATE_SUCCEEDED,
				WorkflowId:    "test-workflow-id",
				WorkflowRunId: "test-run-id",
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
		},
		{
			name: "error getting workflow execution info",
			initialObjects: []client.Object{
				&v2.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineRunSpec{
						Pipeline: &apipb.ResourceIdentifier{
							Name:      "test-pipeline",
							Namespace: "test-namespace",
						},
					},
					Status: v2.PipelineRunStatus{
						WorkflowId:    "test-workflow-id",
						WorkflowRunId: "test-run-id",
						Conditions: []*apipb.Condition{
							{
								Type:   actors.SourcePipelineType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_UNKNOWN,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.SourcePipelineStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							},
						},
						SourcePipeline: &v2.SourcePipeline{
							Pipeline: &v2.Pipeline{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-pipeline",
									Namespace: "test-namespace",
									Annotations: map[string]string{
										pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
									},
								},
								Spec: v2.PipelineSpec{
									Manifest: &v2.PipelineManifest{
										Content:    pipelineManifestContent,
										UniflowTar: "mock://test-uniflow-tar",
									},
								},
							},
						},
						State: v2.PIPELINE_RUN_STATE_RUNNING,
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
						},
					},
					Spec: v2.PipelineSpec{
						Manifest: &v2.PipelineManifest{
							Content:    pipelineManifestContent,
							UniflowTar: "mock://test-uniflow-tar",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				// SourcePipeline.Retrieve() returns TRUE
				// ImageBuild.Retrieve() returns TRUE
				// ExecuteWorkflow.Retrieve() returns FALSE (workflow is running)
				// ExecuteWorkflow.Run() tries to query workflow but gets an error; this is terminal
				mockWorkflowClient.EXPECT().GetWorkflowExecutionInfo(
					gomock.Any(),
					"test-workflow-id",
					"test-run-id",
				).Return(nil, fmt.Errorf("workflow service unavailable"))
			},
			errMsg: "",
			expectedResult: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
				},
			},
			expectedPipelineRunStatus: v2.PipelineRunStatus{
				State:         v2.PIPELINE_RUN_STATE_FAILED,
				WorkflowId:    "test-workflow-id",
				WorkflowRunId: "test-run-id",
			},
			expectedSteps: []*v2.PipelineRunStepInfo{
				{
					Name:  pipelinerunutils.SourcePipelineStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_RUNNING, // Remains RUNNING from initial status since error happens before step update
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctr := gomock.NewController(t)
			defer ctr.Finish()
			mockWorkflowClient := workflowClientMock.NewMockWorkflowClient(ctr)
			mockBlobStorageClient := blobStorageClientMock.NewMockBlobStoreClient(ctr)
			testCase.mockFunc(mockWorkflowClient, mockBlobStorageClient)
			reconciler := setUpReconciler(t, testCase.initialObjects, mockWorkflowClient, mockBlobStorageClient)
			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
			})
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedResult, result)
			pipelineRun := &v2.PipelineRun{}
			reconciler.Get(context.Background(), "test-namespace", "test-pipeline-run", &metav1.GetOptions{}, pipelineRun)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, testCase.expectedConditions, pipelineRun.Status.Conditions)
			require.Equal(t, testCase.expectedPipelineRunStatus.State, pipelineRun.Status.State)
			require.Equal(t, testCase.expectedPipelineRunStatus.WorkflowId, pipelineRun.Status.WorkflowId)
			require.Equal(t, testCase.expectedPipelineRunStatus.WorkflowRunId, pipelineRun.Status.WorkflowRunId)
			for i, step := range pipelineRun.Status.Steps {
				require.Equal(t, testCase.expectedSteps[i].Name, step.Name)
				require.Equal(t, testCase.expectedSteps[i].State, step.State)
			}
		})
	}
}

func setUpReconciler(
	t *testing.T,
	initialObjects []client.Object,
	mockWorkflowClient *workflowClientMock.MockWorkflowClient,
	mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient,
) *Reconciler {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	err := v2.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialObjects...).
		WithStatusSubresource(initialObjects...).
		Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	plugin := plugin.NewPlugin(plugin.PluginParams{
		Logger:         logger,
		WorkflowClient: mockWorkflowClient,
		BlobStore: &blobstore.BlobStore{
			Logger:  logger,
			Clients: map[string]blobstore.BlobStoreClient{"mock": mockBlobStorageClient},
		},
		ApiHandler:     handler,
		ConfigProvider: createMockConfigProvider(),
	})
	// Create a mock notifier to avoid nil pointer dereference
	mockNotifier := notification.NewPipelineRunNotifier(mockWorkflowClient, logger)

	reconciler := &Reconciler{
		Handler:  handler,
		logger:   logger,
		plugin:   plugin,
		engine:   defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		notifier: mockNotifier,
	}

	return reconciler
}

func createMockConfigProvider() uberconfig.Provider {
	configMap := map[string]interface{}{
		"workflowClient": map[string]interface{}{
			"service":   "cadence-frontend",
			"host":      "localhost:7933",
			"transport": "grpc",
			"domain":    "default",
			"taskList":  "default",
		},
	}

	provider, _ := uberconfig.NewStaticProvider(configMap)
	return provider
}
