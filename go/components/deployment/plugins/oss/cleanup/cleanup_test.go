package cleanup

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap/configmapmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy/proxymocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*configmapmocks.MockModelConfigMapProvider, *proxymocks.MockProxyProvider)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
	}{
		{
			name: "model still exists in ConfigMap, cleanup required",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().GetModelsFromConfigMap(gomock.Any(), gomock.Any()).Return(
					[]configmap.ModelConfigEntry{
						{Name: "old-model", S3Path: "s3://bucket/old-model"},
					}, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ModelStillExistsInConfigMap",
			expectedConditionMessage: "Model old-model still exists in ConfigMap",
		},
		{
			name: "unable to check model in ConfigMap",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().GetModelsFromConfigMap(gomock.Any(), gomock.Any()).Return(
					nil, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "UnableToCheckModelInConfigMap",
		},
		{
			name: "HTTPRoute still exists, cleanup required",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().GetModelsFromConfigMap(gomock.Any(), gomock.Any()).Return(
					[]configmap.ModelConfigEntry{}, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "HTTPRouteStillExists",
			expectedConditionMessage: "Cleanup required: HTTPRoute test-deployment-httproute still exists",
		},
		{
			name: "unable to check HTTPRoute exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().GetModelsFromConfigMap(gomock.Any(), gomock.Any()).Return(
					[]configmap.ModelConfigEntry{}, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "UnableToCheckHTTPRouteExists",
		},
		{
			name: "cleanup completed, all resources cleaned up",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().GetModelsFromConfigMap(gomock.Any(), gomock.Any()).Return(
					[]configmap.ModelConfigEntry{}, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Cleanup not required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConfigMap := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockConfigMap, mockProxy)

			actor := &CleanupActor{
				modelConfigMapProvider: mockConfigMap,
				proxyProvider:          mockProxy,
				logger:                 zap.NewNop(),
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

func TestRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*configmapmocks.MockModelConfigMapProvider, *proxymocks.MockProxyProvider)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
		expectedStage            v2pb.DeploymentStage
	}{
		{
			name: "successful cleanup, all operations complete",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Cleanup completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		},
		{
			name: "ConfigMap cleanup fails",
			deployment: &v2pb.Deployment{
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(errors.New("configmap update failed"))
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "ConfigMapCleanupFailed",
			expectedConditionMessage: "Failed to remove old model old-model from ConfigMap",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		},
		{
			name: "HTTPRoute deletion fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("deletion failed"))
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "HTTPRouteCleanupFailed",
			expectedConditionMessage: "Failed to delete HTTPRoute test-deployment-httproute",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		},
		{
			name: "HTTPRoute not found during deletion, continues successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			},
			setupMocks: func(mcp *configmapmocks.MockModelConfigMapProvider, pp *proxymocks.MockProxyProvider) {
				mcp.EXPECT().RemoveModelFromConfigMap(gomock.Any(), gomock.Any()).Return(nil)
				notFoundErr := kerrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "test-deployment-httproute")
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFoundErr)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "CleanupCompleted",
			expectedConditionMessage: "Cleanup completed successfully",
			expectedStage:            v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConfigMap := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockConfigMap, mockProxy)

			actor := &CleanupActor{
				modelConfigMapProvider: mockConfigMap,
				proxyProvider:          mockProxy,
				logger:                 zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			if tt.expectedConditionMessage != "" {
				assert.Contains(t, condition.Message, tt.expectedConditionMessage)
			}
			assert.Equal(t, tt.expectedStage, tt.deployment.Status.Stage)
		})
	}
}
