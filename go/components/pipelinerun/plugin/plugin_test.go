package plugin

import (
	"testing"

	"github.com/golang/mock/gomock"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	blobstoreMock "github.com/michelangelo-ai/michelangelo/go/base/blobstore/blobstore_mocks"
	workflowclientMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
	uberconfig "go.uber.org/config"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetConditions(t *testing.T) {
	testCases := []struct {
		name               string
		pipelineRun        *v2.PipelineRun
		expectedConditions []*apipb.Condition
	}{
		{
			name: "Get All conditions",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   actors.SourcePipelineType,
							Status: apipb.CONDITION_STATUS_FALSE,
						},
						{
							Type:   actors.ImageBuildType,
							Status: apipb.CONDITION_STATUS_FALSE,
						},
					},
				},
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
				{
					Type:   actors.ImageBuildType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
			},
		},
		{
			name: "Return empty conditions if no conditions are present",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{},
			},
			expectedConditions: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			plugin := setupPlugin(t)
			conditions := plugin.GetConditions(testCase.pipelineRun)
			require.Equal(t, testCase.expectedConditions, conditions)
		})
	}
}

func TestPutCondition(t *testing.T) {
	testCases := []struct {
		name               string
		pipelineRun        *v2.PipelineRun
		condition          *apipb.Condition
		expectedConditions []*apipb.Condition
	}{
		{
			name: "Add condition if it doesn't exist",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{},
			},
			condition: &apipb.Condition{
				Type:   actors.SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
			},
		},
		{
			name: "Update condition if it exists",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   actors.SourcePipelineType,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
					},
				},
			},
			condition: &apipb.Condition{
				Type:   actors.SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedConditions: []*apipb.Condition{
				{
					Type:   actors.SourcePipelineType,
					Status: apipb.CONDITION_STATUS_FALSE,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			plugin := setupPlugin(t)
			plugin.PutCondition(testCase.pipelineRun, testCase.condition)
			require.Equal(t, testCase.expectedConditions, testCase.pipelineRun.Status.Conditions)
		})
	}
}

func setupPlugin(t *testing.T) *Plugin {
	scheme := runtime.NewScheme()
	err := v2.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := apiHandler.NewFakeAPIHandler(k8sClient)
	ctl := gomock.NewController(t)
	mockWorkflowClient := workflowclientMock.NewMockWorkflowClient(ctl)
	mockBlobStoreClient := blobstoreMock.NewMockBlobStoreClient(ctl)
	logger := zaptest.NewLogger(t)
	blobStore := blobstore.BlobStore{
		Logger:  logger,
		Clients: map[string]blobstore.BlobStoreClient{"mock": mockBlobStoreClient},
	}

	// Create a mock config provider for testing
	configProvider, err := uberconfig.NewYAML(uberconfig.Static(map[string]interface{}{
		"workflowClient": map[string]interface{}{
			"taskList": "default",
		},
	}))
	require.NoError(t, err)

	plugin := NewPlugin(PluginParams{
		ApiHandler:     handler,
		WorkflowClient: mockWorkflowClient,
		BlobStore:      &blobStore,
		ConfigProvider: configProvider,
		Logger:         logger,
	})
	return plugin
}
