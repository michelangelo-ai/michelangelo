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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*gatewaysmocks.MockGateway, *proxymocks.MockRouteProvider)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().CheckModelExists(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(true, nil)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().CheckModelExists(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if model old-model exists in Inference Server: connection error",
		},
		{
			name: "HTTPRoute still exists, cleanup required",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().CheckModelExists(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Cleanup required: HTTPRoute test-deployment-httproute still exists",
		},
		{
			name: "unable to check HTTPRoute exists",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().CheckModelExists(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if HTTPRoute test-deployment-httproute exists: api error",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().CheckModelExists(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(false, nil)
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(false, nil)
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
			mockProxy := proxymocks.NewMockRouteProvider(ctrl)

			tt.setupMocks(mockModelConfigProvider, mockProxy)

			actor := &CleanupActor{
				modelConfigProvider: mockModelConfigProvider,
				RouteProvider:       mockProxy,
				logger:              zap.NewNop(),
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
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider, *proxymocks.MockRouteProvider)
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(nil)
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "model unloading fails",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(errors.New("unload failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to unload old model old-model from inference server: unload failed",
		},
		{
			name: "HTTPRoute deletion fails with error",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(nil)
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(errors.New("deletion failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to delete HTTPRoute test-deployment-httproute: deletion failed",
		},
		{
			name: "HTTPRoute not found during deletion, continues successfully",
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
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockRouteProvider) {
				gw.EXPECT().UnloadModel(gomock.Any(), gomock.Any(), "old-model", "test-server", "default", v2pb.BACKEND_TYPE_TRITON).Return(nil)
				notFoundErr := kerrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "test-deployment-httproute")
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(notFoundErr)
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
			mockProxy := proxymocks.NewMockRouteProvider(ctrl)

			tt.setupMocks(mockModelConfigProvider, mockProxy)

			actor := &CleanupActor{
				modelConfigProvider: mockModelConfigProvider,
				RouteProvider:       mockProxy,
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
