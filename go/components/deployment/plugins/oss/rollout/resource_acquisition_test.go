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
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestResourceAcquisitionRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
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
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
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
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "No inference server specified for deployment",
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
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Inference server is not healthy",
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
				gw.EXPECT().InferenceServerIsHealthy(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check health of inference server: connection error",
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

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
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
