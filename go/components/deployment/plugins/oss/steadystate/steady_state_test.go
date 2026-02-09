package steadystate

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*gatewaysmocks.MockGateway)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, errors.New("connection error"))
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check model status: api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &SteadyStateActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
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
