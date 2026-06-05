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

	"github.com/michelangelo-ai/michelangelo/go/api"
	blobStorageClientMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowClientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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

func TestMarkImmutableIfExpired(t *testing.T) {
	now := time.Now()
	twoDaysAgo := metav1.NewTime(now.Add(-48 * time.Hour))
	oneDayAgo := metav1.NewTime(now.Add(-24 * time.Hour))

	tests := []struct {
		name                      string
		pipelineRun               *v2.PipelineRun
		ttlDays                   int
		expectedImmutable         bool
		expectedRequeue           bool
		expectedRequeueAfterRange struct{ min, max time.Duration }
	}{
		{
			name: "TTL not elapsed - should requeue",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pr",
					Namespace:         "test-ns",
					CreationTimestamp: oneDayAgo,
				},
				Status: v2.PipelineRunStatus{
					State:   v2.PIPELINE_RUN_STATE_SUCCEEDED,
					EndTime: &pbtypes.Timestamp{Seconds: oneDayAgo.Unix()},
				},
			},
			ttlDays:           2, // TTL is 2 days, but only 1 day has passed
			expectedImmutable: false,
			expectedRequeue:   true,
			expectedRequeueAfterRange: struct{ min, max time.Duration }{
				min: 23*time.Hour + 59*time.Minute, // ~1 day remaining
				max: 24*time.Hour + 1*time.Minute,
			},
		},
		{
			name: "TTL elapsed - should mark immutable",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pr",
					Namespace:         "test-ns",
					CreationTimestamp: twoDaysAgo,
				},
				Status: v2.PipelineRunStatus{
					State:   v2.PIPELINE_RUN_STATE_SUCCEEDED,
					EndTime: &pbtypes.Timestamp{Seconds: twoDaysAgo.Unix()},
				},
			},
			ttlDays:           1, // TTL is 1 day, but 2 days have passed
			expectedImmutable: true,
			expectedRequeue:   false,
		},
		{
			name: "TTL elapsed - already immutable",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pr",
					Namespace:         "test-ns",
					CreationTimestamp: twoDaysAgo,
					Annotations: map[string]string{
						api.ImmutableAnnotation: "true",
					},
				},
				Status: v2.PipelineRunStatus{
					State:   v2.PIPELINE_RUN_STATE_SUCCEEDED,
					EndTime: &pbtypes.Timestamp{Seconds: twoDaysAgo.Unix()},
				},
			},
			ttlDays:           1,
			expectedImmutable: true,
			expectedRequeue:   false,
		},
		{
			name: "No EndTime - falls back to creation timestamp",
			pipelineRun: &v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pr",
					Namespace:         "test-ns",
					CreationTimestamp: twoDaysAgo,
				},
				Status: v2.PipelineRunStatus{
					State: v2.PIPELINE_RUN_STATE_SUCCEEDED,
					// EndTime is nil
				},
			},
			ttlDays:           1, // TTL is 1 day, creation was 2 days ago
			expectedImmutable: true,
			expectedRequeue:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.pipelineRun).
				WithStatusSubresource(&v2.PipelineRun{}).
				Build()

			handler := apiHandler.NewFakeAPIHandler(k8sClient)

			reconciler := &Reconciler{
				Handler: handler,
				logger:  logger,
				config: Config{
					TTLDays: tt.ttlDays,
				},
				engine: defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
			}

			requeueAfter, done := reconciler.markImmutableIfExpired(
				context.Background(),
				logger,
				tt.pipelineRun,
			)

			// Check requeue behavior
			if tt.expectedRequeue {
				require.False(t, done, "Expected not done (should requeue)")
				require.Greater(t, requeueAfter, tt.expectedRequeueAfterRange.min, "Requeue time too short")
				require.Less(t, requeueAfter, tt.expectedRequeueAfterRange.max, "Requeue time too long")
			} else {
				require.True(t, done, "Expected done (should not requeue)")
				require.Equal(t, time.Duration(0), requeueAfter, "Expected no requeue delay")
			}

			// Check if immutable annotation was set correctly
			updatedPR := &v2.PipelineRun{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      tt.pipelineRun.Name,
				Namespace: tt.pipelineRun.Namespace,
			}, updatedPR)
			require.NoError(t, err)

			annotations := updatedPR.GetAnnotations()
			if tt.expectedImmutable {
				require.Equal(t, "true", annotations[api.ImmutableAnnotation], "Expected immutable annotation to be set")
			} else {
				require.NotEqual(t, "true", annotations[api.ImmutableAnnotation], "Expected immutable annotation to not be set")
			}
		})
	}
}

func TestReconcileTTLWithMetadataStorageDisabled(t *testing.T) {
	now := time.Now()
	threeDaysAgo := metav1.NewTime(now.Add(-72 * time.Hour))

	tests := []struct {
		name                  string
		metadataStorageConfig storage.MetadataStorageConfig
		ttlDays               int
		shouldMarkImmutable   bool
	}{
		{
			name: "Metadata storage enabled - TTL should work",
			metadataStorageConfig: storage.MetadataStorageConfig{
				EnableMetadataStorage: true,
			},
			ttlDays:             1,
			shouldMarkImmutable: true,
		},
		{
			name: "Metadata storage disabled - TTL should be skipped",
			metadataStorageConfig: storage.MetadataStorageConfig{
				EnableMetadataStorage: false,
			},
			ttlDays:             1,
			shouldMarkImmutable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineRun := &v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pr-ttl",
					Namespace:         "test-ns",
					CreationTimestamp: threeDaysAgo,
				},
				Status: v2.PipelineRunStatus{
					State:   v2.PIPELINE_RUN_STATE_SUCCEEDED,
					EndTime: &pbtypes.Timestamp{Seconds: threeDaysAgo.Unix()},
				},
			}

			logger := zaptest.NewLogger(t)
			scheme := runtime.NewScheme()
			err := v2.AddToScheme(scheme)
			require.NoError(t, err)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pipelineRun).
				WithStatusSubresource(&v2.PipelineRun{}).
				Build()

			handler := apiHandler.NewFakeAPIHandler(k8sClient)

			reconciler := &Reconciler{
				Handler: handler,
				logger:  logger,
				config: Config{
					TTLDays: tt.ttlDays,
				},
				metadataStorageEnabled: storage.EnableMetadataStorage(&tt.metadataStorageConfig),
				engine:                 defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
				// Empty actors list: engine immediately returns terminal+satisfied,
				// exercising the TTL branch without requiring workflow/blob dependencies.
				plugin:   &plugin.Plugin{},
				notifier: notification.NewPipelineRunNotifier(nil, logger),
			}

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      pipelineRun.Name,
					Namespace: pipelineRun.Namespace,
				},
			})
			require.NoError(t, err)

			// Verify the result
			updatedPR := &v2.PipelineRun{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      pipelineRun.Name,
				Namespace: pipelineRun.Namespace,
			}, updatedPR)
			require.NoError(t, err)

			annotations := updatedPR.GetAnnotations()
			if tt.shouldMarkImmutable {
				require.Equal(t, "true", annotations[api.ImmutableAnnotation],
					"Expected immutable annotation when metadata storage is enabled")
				require.Equal(t, time.Duration(0), result.RequeueAfter,
					"Should not requeue after marking immutable")
			} else {
				require.NotEqual(t, "true", annotations[api.ImmutableAnnotation],
					"Expected NO immutable annotation when metadata storage is disabled (prevents data loss)")
				require.Equal(t, time.Duration(0), result.RequeueAfter,
					"Should not requeue when TTL is skipped")
			}
		})
	}
}
