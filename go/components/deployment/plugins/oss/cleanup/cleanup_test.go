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

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route/routemocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider, *routemocks.MockRouteProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "model still exists in inference server, cleanup required",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return([]modelconfig.ModelConfigEntry{
					{Name: "old-model", StoragePath: "gs://bucket/old-model"},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Model old-model still exists in Inference Server",
		},
		{
			name: "unable to check model in inference server",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return(nil, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if model old-model exists in Inference Server: connection error",
		},
		{
			name: "DeploymentRoute still exists, cleanup required",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				// Model doesn't exist but route still exists
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return([]modelconfig.ModelConfigEntry{}, nil)
				rp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Cleanup required: DeploymentRoute test-deployment still exists",
		},
		{
			name: "unable to check DeploymentRoute exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return([]modelconfig.ModelConfigEntry{}, nil)
				rp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if DeploymentRoute exists for deployment test-deployment: api error",
		},
		{
			name: "cleanup completed, all resources cleaned up",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: "old-model"},
				},
			},
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").Return([]modelconfig.ModelConfigEntry{}, nil)
				rp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			mockRouteProvider := routemocks.NewMockRouteProvider(ctrl)

			tt.setupMocks(mockModelConfigProvider, mockRouteProvider)

			actor := &CleanupActor{
				ModelConfigProvider: mockModelConfigProvider,
				RouteProvider:       mockRouteProvider,
				Logger:              zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Contains(t, condition.Reason, tt.expectedConditionReason)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider, *routemocks.MockRouteProvider)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "successful cleanup, all operations complete",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
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
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "old-model").Return(nil)
				rp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "model removal fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
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
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "old-model").Return(errors.New("removal failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to unload old model old-model from inference server: removal failed",
		},
		{
			name: "DeploymentRoute deletion fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
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
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "old-model").Return(nil)
				rp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(errors.New("deletion failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to delete DeploymentRoute",
		},
		{
			name: "DeploymentRoute not found during deletion, continues successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
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
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rp *routemocks.MockRouteProvider) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default", "old-model").Return(nil)
				notFoundErr := kerrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "test-deployment-httproute")
				rp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any(), "test-deployment", "default").Return(notFoundErr)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			mockRouteProvider := routemocks.NewMockRouteProvider(ctrl)

			tt.setupMocks(mockModelConfigProvider, mockRouteProvider)

			actor := &CleanupActor{
				ModelConfigProvider: mockModelConfigProvider,
				RouteProvider:       mockRouteProvider,
				Logger:              zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Contains(t, condition.Reason, tt.expectedConditionReason)
		})
	}
}
