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
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "steady state reached when rollout complete and healthy",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateReached",
			expectedConditionMessage: "Deployment is in steady state",
		},
		{
			name: "steady state restored when rollback complete",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateRestored",
			expectedConditionMessage: "Deployment has been restored to steady state",
		},
		{
			name: "not in steady state when rollout complete but unhealthy",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NotInSteadyState",
			expectedConditionMessage: "Deployment not yet in steady state",
		},
		{
			name: "not in steady state when in validation stage",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_VALIDATION,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NotInSteadyState",
			expectedConditionMessage: "Deployment not yet in steady state",
		},
		{
			name: "not in steady state when in placement stage",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_PLACEMENT,
					State: v2pb.DEPLOYMENT_STATE_HEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NotInSteadyState",
			expectedConditionMessage: "Deployment not yet in steady state",
		},
		{
			name: "not in steady state when rollback in progress",
			deployment: &v2pb.Deployment{
				Status: v2pb.DeploymentStatus{
					Stage: v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
					State: v2pb.DEPLOYMENT_STATE_UNHEALTHY,
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NotInSteadyState",
			expectedConditionMessage: "Deployment not yet in steady state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &SteadyStateActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*gatewaysmocks.MockGateway)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
		expectedState            v2pb.DeploymentState
	}{
		{
			name: "steady state maintained when rollout complete and healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - healthy state, no checks performed
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateReached",
			expectedConditionMessage: "Deployment is in steady state",
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
		},
		{
			name: "health check restores deployment to healthy state",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateReached",
			expectedConditionMessage: "Deployment is in steady state",
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
		},
		{
			name: "health check fails when inference server unhealthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "HealthCheckFailed",
			expectedConditionMessage: "Inference server is not healthy",
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "health check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "HealthCheckFailed",
			expectedState:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "model status check fails when model not ready",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ModelHealthCheckFailed",
			expectedConditionMessage: "Model is not ready",
			expectedState:            v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "model status check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().IsHealthy(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "ModelHealthCheckFailed",
			expectedState:           v2pb.DEPLOYMENT_STATE_UNHEALTHY,
		},
		{
			name: "steady state reached when not in rollout complete stage",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - not in rollout complete stage
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateReached",
			expectedConditionMessage: "Deployment is in steady state",
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
		},
		{
			name: "revision mismatch detected but steady state reached",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
					State:           v2pb.DEPLOYMENT_STATE_HEALTHY,
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v1"},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - healthy state, just logs mismatch
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "SteadyStateReached",
			expectedConditionMessage: "Deployment is in steady state",
			expectedState:            v2pb.DEPLOYMENT_STATE_HEALTHY,
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

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			if tt.expectedConditionMessage != "" {
				assert.Contains(t, condition.Message, tt.expectedConditionMessage)
			}
			assert.Equal(t, tt.expectedState, tt.deployment.Status.State)
		})
	}
}
