package pipelinerun

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"

	blobStorageClientMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	workflowClientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcile(t *testing.T) {
	encodedContent := "Cix0eXBlLmdvb2dsZWFwaXMuY29tL21pY2hlbGFuZ2Vsby5VbmlGbG93Q29uZhLlBQqwAgoMZmVhdHVyZV9wcmVwEp8CKpwCChEKBHNlZWQSCREAAAAAAADwPwptCg5oaXZlX3RhYmxlX3VybBJbGlloZGZzOi8vL3VzZXIvaGl2ZS93YXJlaG91c2UvbWljaGVsYW5nZWxvLmRiL2RsX2V4YW1wbGVfZGF0YXNldHNfYm9zdG9uX2hvdXNpbmdfZnA2NF9sYWJlbAp+Cg9mZWF0dXJlX2NvbHVtbnMSazJpCgUaA2FnZQoDGgFiCgYaBGNoYXMKBhoEY3JpbQoFGgNkaXMKBxoFaW5kdXMKBxoFbHN0YXQKBRoDbm94CgkaB3B0cmF0aW8KBRoDcmFkCgQaAnJtCgUaA3RheAoEGgJ6bgoGGgRtZWR2ChgKC3RyYWluX3JhdGlvEgkRmpmZmZmZ6T8KVQoRd29ya2Zsb3dfZnVuY3Rpb24SQBo+dWJlci5haS5taWNoZWxhbmdlbG8uZXhwZXJpbWVudGFsLm1hZi53b3JrZmxvdy5UcmFpblNpbXBsaWZpZWQKvwEKBXRyYWluErUBKrIBCq8BCgp4Z2JfcGFyYW1zEqABKp0BChkKCW9iamVjdGl2ZRIMGgpyZWc6bGluZWFyChkKDG5fZXN0aW1hdG9ycxIJEQAAAAAAACRAChYKCW1heF9kZXB0aBIJEQAAAAAAABRAChoKDWxlYXJuaW5nX3JhdGUSCRGamZmZmZm5PwodChBjb2xzYW1wbGVfYnl0cmVlEgkRMzMzMzMz0z8KEgoFYWxwaGESCREAAAAAAAAkQAqWAQoKcHJlcHJvY2VzcxKHASqEAQqBAQoSY2FzdF9mbG9hdF9jb2x1bW5zEmsyaQoFGgNhZ2UKAxoBYgoGGgRjaGFzCgYaBGNyaW0KBRoDZGlzCgcaBWluZHVzCgcaBWxzdGF0CgUaA25veAoJGgdwdHJhdGlvCgUaA3JhZAoEGgJybQoFGgN0YXgKBBoCem4KBhoEbWVkdg=="
	contentStr, _ := base64.StdEncoding.DecodeString(encodedContent)
	pipelineManifestContet := &pbtypes.Any{
		Value:   contentStr,
		TypeUrl: "type.googleapis.com/michelangelo.api.TypedStruct",
	}
	testCases := []struct {
		name                      string
		initialObjects            []runtime.Object
		mockFunc                  func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient)
		expectedConditions        []*apipb.Condition
		expectedPipelineRunStatus v2.PipelineRunStatus
		expectedSteps             []*v2.PipelineRunStepInfo
		errMsg                    string
		expectedResult            ctrl.Result
	}{
		{
			name: "All conditions are nil. Initial pipeline run condition and steps are added",
			initialObjects: []runtime.Object{
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
				// Do nothing
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
				},
				{
					Type:   actors.ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_UNKNOWN,
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
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
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
			name: "Source pipeline condition is true",
			initialObjects: []runtime.Object{
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
								Status: apipb.CONDITION_STATUS_UNKNOWN,
							},
							{
								Type:   actors.ImageBuildType,
								Status: apipb.CONDITION_STATUS_UNKNOWN,
							},
							{
								Type:   actors.ExecuteWorkflowType,
								Status: apipb.CONDITION_STATUS_UNKNOWN,
							},
						},
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
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
							Content:    pipelineManifestContet,
							UniflowTar: "mock://test-uniflow-tar",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				mockBlobStorageClient.EXPECT().Get(gomock.Any(), "mock://test-uniflow-tar").Return([]byte("test-content"), nil)
				mockWorkflowClient.EXPECT().StartWorkflow(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					&clientInterfaces.WorkflowExecution{
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
			name: "Workflow is succeeded",
			initialObjects: []runtime.Object{
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
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							},
						},
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
					},
				},
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
					Spec: v2.PipelineSpec{
						Manifest: &v2.PipelineManifest{
							Content:    pipelineManifestContet,
							UniflowTar: "test-uniflow-tar",
						},
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				mockWorkflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(
					&clientInterfaces.WorkflowExecutionInfo{
						Status: clientInterfaces.WorkflowExecutionStatusCompleted,
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
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
			},
			errMsg:         "",
			expectedResult: ctrl.Result{},
		},
		{
			name: "Error getting workflow execution info",
			initialObjects: []runtime.Object{
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
						Steps: []*v2.PipelineRunStepInfo{
							{
								Name:  pipelinerunutils.ImageBuildStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
							},
							{
								Name:  pipelinerunutils.ExecuteWorkflowStepName,
								State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
							},
						},
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
						State: v2.PIPELINE_RUN_STATE_RUNNING,
					},
				},
			},
			mockFunc: func(mockWorkflowClient *workflowClientMock.MockWorkflowClient, mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient) {
				mockWorkflowClient.EXPECT().GetWorkflowExecutionInfo(gomock.Any(), "test-workflow-id", "test-run-id").Return(
					nil, fmt.Errorf("test error"))
			},
			errMsg: "test error",
			expectedResult: ctrl.Result{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
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
					Name:  pipelinerunutils.ImageBuildStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				},
				{
					Name:  pipelinerunutils.ExecuteWorkflowStepName,
					State: v2.PIPELINE_RUN_STEP_STATE_RUNNING,
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
	initialObjects []runtime.Object,
	mockWorkflowClient *workflowClientMock.MockWorkflowClient,
	mockBlobStorageClient *blobStorageClientMock.MockBlobStoreClient,
) *Reconciler {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	err := v2.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialObjects...).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	plugin := plugin.NewPlugin(plugin.PluginParams{
		Logger:         logger,
		WorkflowClient: mockWorkflowClient,
		BlobStore: &blobstore.BlobStore{
			Logger:  logger,
			Clients: map[string]blobstore.BlobStoreClient{"mock": mockBlobStorageClient},
		},
		ApiHandler: handler,
	})
	reconciler := &Reconciler{
		Handler: handler,
		logger:  logger,
		plugin:  plugin,
		engine:  defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
	}

	return reconciler
}
