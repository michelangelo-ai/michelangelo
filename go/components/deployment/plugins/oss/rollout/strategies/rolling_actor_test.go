package strategies

import (
	"context"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRollingRolloutRetrieve(t *testing.T) {
	// Condition with rolloutstarted = true metadata
	rolloutStartedCondition := func() *api.Condition {
		rolloutstarted := &types.BoolValue{Value: true}
		metadata, _ := types.MarshalAny(rolloutstarted)
		return &api.Condition{Metadata: metadata}
	}

	// Condition with rolloutstarted = false metadata
	rolloutNotStartedCondition := func() *api.Condition {
		return &api.Condition{}
	}

	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		condition               *api.Condition
		setupMocks              func(*gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "rolling rollout not started when metadata not set",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			condition:               rolloutNotStartedCondition(),
			setupMocks:              func(gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Rolling rollout has not started",
		},
		{
			name: "model sync completed when model is ready",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			condition: rolloutStartedCondition(),
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &RollingRolloutActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, tt.condition)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestRollingRolloutRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "successful model sync via model config provider",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", modelconfig.ModelConfigEntry{Name: "model-v1", StoragePath: "s3://deploy-models/model-v1/"}).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason: "Rolling rollout is in progress",
		},
		{
			name: "model loading fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", modelconfig.ModelConfigEntry{Name: "model-v2", StoragePath: "s3://deploy-models/model-v2/"}).Return(errors.New("model loading failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to update deployment: model loading failed",
		},
		{
			name: "model sync without desired revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks:              func(mcp *modelconfigmocks.MockModelConfigProvider) {},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason: "Rolling rollout is in progress",
		},
		{
			name: "successful model sync for bert_cola",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "bert-deployment", Namespace: "production"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-prod"},
					},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider) {
				mcp.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), "triton-prod", "production", modelconfig.ModelConfigEntry{Name: "bert_cola", StoragePath: "s3://deploy-models/bert_cola/"}).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason: "Rolling rollout is in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			tt.setupMocks(mockModelConfigProvider)

			actor := &RollingRolloutActor{
				modelConfigProvider: mockModelConfigProvider,
				logger:              zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}
