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

func TestModelCleanupRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*gatewaysmocks.MockGateway)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "NoCleanupNeeded",
			expectedConditionMessage: "No cleanup required",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				// No mocks needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "NoCleanupNeeded",
			expectedConditionMessage: "No cleanup required",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CleanupPending",
			expectedConditionMessage: "Old model model-v1 still loaded, cleanup needed",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupComplete",
			expectedConditionMessage: "Old model model-v1 already cleaned up",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("api error"))
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "CleanupPending",
			expectedConditionMessage: "Need to cleanup old model model-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &ModelCleanupActor{
				Gateway: mockGateway,
				Logger:  zap.NewNop(),
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

func TestModelCleanupRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*configmapmocks.MockModelConfigMapProvider, *gatewaysmocks.MockGateway)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, gw *gatewaysmocks.MockGateway) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Successfully cleaned up old model model-v1",
		},
		{
			name: "cleanup continues even if ConfigMap removal fails",
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, gw *gatewaysmocks.MockGateway) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(errors.New("configmap error"))
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Successfully cleaned up old model model-v1",
		},
		{
			name: "cleanup continues even if unload fails",
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, gw *gatewaysmocks.MockGateway) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("unload failed"))
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Successfully cleaned up old model model-v1",
		},
		{
			name: "cleanup completes with verification showing model unloaded",
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
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, gw *gatewaysmocks.MockGateway) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Successfully cleaned up old model old-bert",
		},
		{
			name: "cleanup completes even when verification check fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v3"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "model-v2"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, gw *gatewaysmocks.MockGateway) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				gw.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("verification failed"))
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Successfully cleaned up old model model-v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConfigMap := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockConfigMap, mockGateway)

			actor := &ModelCleanupActor{
				ModelConfigMapProvider: mockConfigMap,
				Gateway:                mockGateway,
				Logger:                 zap.NewNop(),
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
