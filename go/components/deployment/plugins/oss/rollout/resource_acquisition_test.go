package rollout

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func createTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	return registry
}

func TestResourceAcquisitionRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*backendsmocks.MockBackend)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
		expectError             bool
	}{
		{
			name: "resources available when inference server is healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
			expectError:             false,
		},
		{
			name: "no inference server specified",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: nil,
				},
			},
			setupMocks:              func(mb *backendsmocks.MockBackend) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "No inference server specified for deployment",
			expectError:             false,
		},
		{
			name: "inference server is not healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Inference server is not healthy",
			expectError:             false,
		},
		{
			name: "health check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check health of inference server: connection error",
			expectError:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := &ResourceAcquisitionActor{
				backendRegistry: createTestRegistry(mockBackend),
				logger:          zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestResourceAcquisitionRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		inputCondition          *api.Condition
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "run returns the input condition unchanged",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			inputCondition: &api.Condition{
				Status: api.CONDITION_STATUS_FALSE,
				Reason: "ResourcesNotAvailable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &ResourceAcquisitionActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, tt.inputCondition)

			assert.NoError(t, err)
			assert.Equal(t, tt.inputCondition, condition)
		})
	}
}
