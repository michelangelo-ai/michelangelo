package steadystate

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

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*backendsmocks.MockBackend)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "steady state reached when inference server and model are healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "not in steady state when inference server is unhealthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Inference server is not healthy",
		},
		{
			name: "not in steady state when inference server health check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check health of inference server: connection error",
		},
		{
			name: "not in steady state when model is not ready",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Model is not ready",
		},
		{
			name: "not in steady state when model status check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			setupMocks: func(mb *backendsmocks.MockBackend) {
				mb.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(true, nil)
				mb.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "model-v1").Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check model status: api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := &SteadyStateActor{
				backendRegistry: createTestRegistry(mockBackend),
				logger:          zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestRun(t *testing.T) {
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
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			inputCondition: &api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
				Reason: "TestReason",
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "TestReason",
		},
		{
			name: "run preserves false condition",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			inputCondition: &api.Condition{
				Status: api.CONDITION_STATUS_FALSE,
				Reason: "HealthCheckFailed",
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "HealthCheckFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &SteadyStateActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, tt.inputCondition)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}
