package strategies

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap/configmapmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestModelSyncRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*gatewaysmocks.MockGateway)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "ModelSyncCompleted",
			expectedConditionMessage: "Model model-v1 successfully loaded and ready in Triton",
		},
		{
			name: "model sync pending when no desired revision",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: nil,
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ModelSyncPending",
			expectedConditionMessage: "Model sync is in progress",
		},
		{
			name: "model sync completed for bert_cola model",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "bert-deployment", Namespace: "production"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-prod"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "ModelSyncCompleted",
			expectedConditionMessage: "Model bert_cola successfully loaded and ready in Triton",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &ModelSyncActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Contains(t, condition.Message, tt.expectedConditionMessage)
		})
	}
}

func TestModelSyncRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*configmapmocks.MockModelConfigMapProvider)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "successful model sync to configmap",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider) {
				mcp.EXPECT().AddModelToConfigMap(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason:  "ModelSyncPending",
			expectedConditionMessage: "Model sync is in progress",
		},
		{
			name: "configmap update fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v2"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider) {
				mcp.EXPECT().AddModelToConfigMap(gomock.Any(), gomock.Any()).Return(errors.New("configmap update failed"))
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ConfigMapUpdateFailed",
			expectedConditionMessage: "Failed to update deployment",
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider) {
				// No mocks needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason:  "ModelSyncPending",
			expectedConditionMessage: "Model sync is in progress",
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider) {
				mcp.EXPECT().AddModelToConfigMap(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason:  "ModelSyncPending",
			expectedConditionMessage: "Model sync is in progress",
		},
		{
			name: "model sync with complex deployment",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-deployment",
					Namespace: "staging",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/strategy": "rolling",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "llm-model"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-staging"},
					},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider) {
				mcp.EXPECT().AddModelToConfigMap(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedConditionReason:  "ModelSyncPending",
			expectedConditionMessage: "Model sync is in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConfigMap := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			tt.setupMocks(mockConfigMap)

			actor := &ModelSyncActor{
				modelConfigMapProvider: mockConfigMap,
				logger:                 zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			assert.Contains(t, condition.Message, tt.expectedConditionMessage)
		})
	}
}
