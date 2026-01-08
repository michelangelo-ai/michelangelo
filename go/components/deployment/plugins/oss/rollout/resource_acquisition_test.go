package rollout

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

func TestResourceAcquisitionRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*gatewaysmocks.MockGateway)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "ResourcesAvailable",
			expectedConditionMessage: "Resources are available and can be acquired",
		},
		{
			name: "no inference server specified",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: nil,
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mock setup needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "NoInferenceServer",
			expectedConditionMessage: "No inference server specified for deployment",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "HealthCheckFailed",
			expectedConditionMessage: "Inference server is not healthy",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "HealthCheckFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &ResourceAcquisitionActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			if tt.expectedConditionMessage != "" {
				assert.Contains(t, condition.Message, tt.expectedConditionMessage)
			}
		})
	}
}

func TestResourceAcquisitionRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "resources not available returns terminal state",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ResourcesNotAvailable",
			expectedConditionMessage: "Resources are not available and cannot be acquired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &ResourceAcquisitionActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Equal(t, tt.expectedConditionMessage, condition.Message)
		})
	}
}
