package strategies

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestModelCleanupRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*backendsmocks.MockBackend)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "no cleanup needed when current model is empty",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: ""},
				},
			},
			setupMocks:              func(mb *backendsmocks.MockBackend) {},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "no cleanup needed when models are the same",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks:              func(mb *backendsmocks.MockBackend) {},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "cleanup pending when old model still loaded",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Old model model-v1 still loaded, cleanup needed",
		},
		{
			name: "cleanup complete when old model not found",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "cleanup pending when cannot verify model status",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Need to cleanup old model model-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := &ModelCleanupActor{
				BackendRegistry: createTestRegistry(mockBackend),
				Logger:          zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestModelCleanupRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "successful cleanup of old model",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "cleanup fails when remove model fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(errors.New("removal error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to remove old model model-v1 from model config: removal error",
		},
		{
			name: "cleanup successful with different model names",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "production"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-prod"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-bert"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "triton-prod", "production", "old-bert").Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			tt.setupMocks(mockModelConfigProvider)

			actor := &ModelCleanupActor{
				ModelConfigProvider: mockModelConfigProvider,
				Logger:              zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}
