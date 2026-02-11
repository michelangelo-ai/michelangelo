package cleanup

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy/proxymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestCleanupActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *apipb.Condition
		setupMocks      func(*gatewaysmocks.MockGateway, *proxymocks.MockProxyProvider)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "no metadata returns CleanupNotStarted",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition:       func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "CleanupNotStarted",
		},
		{
			name: "all clusters cleaned and HTTPRoute deleted",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStateCleaned},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "all clusters cleaned but HTTPRoute still exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStateCleaned},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().DeploymentRouteExists(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HTTPRouteStillExists",
		},
		{
			name: "cluster pending triggers Run",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "DeletionCleanupPending",
		},
		{
			name: "cleanup in progress and model still exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStateCleanupInProgress},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				gw.EXPECT().CheckModelExists(
					gomock.Any(), gomock.Any(), "old-model", "test-server", "default", gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DeletionInProgress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			tt.setupMocks(mockGateway, mockProxy)

			actor := &CleanupActor{
				gateway:       mockGateway,
				proxyProvider: mockProxy,
				logger:        zap.NewNop(),
			}

			result, err := actor.Retrieve(context.Background(), tt.deployment, tt.condition())

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, result.Message)
			}
		})
	}
}

func TestCleanupActor_Run(t *testing.T) {
	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *apipb.Condition
		setupMocks      func(*gatewaysmocks.MockGateway, *proxymocks.MockProxyProvider)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "initializes metadata from inference server",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType: v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{
							{ClusterId: "cluster-1", Host: "host1"},
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "MetadataInitialized",
		},
		{
			name: "unloads model from pending cluster",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "old-model", "test-server", "default", gomock.Any(),
				).Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "DeletionStarted",
		},
		{
			name: "unload model fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "old-model", "test-server", "default", gomock.Any(),
				).Return(errors.New("unload failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "ModelUnloadingFailed",
		},
		{
			name: "all clusters cleaned deletes HTTPRoute",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStateCleaned},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "HTTPRoute deletion fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStateCleaned},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(errors.New("delete failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HTTPRouteCleanupFailed",
		},
		{
			name: "HTTPRoute not found continues successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &apipb.ResourceIdentifier{Name: "old-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterId: "cluster-1", State: common.ClusterStateCleaned},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway, pp *proxymocks.MockProxyProvider) {
				notFoundErr := kerrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "test-deployment-httproute")
				pp.EXPECT().DeleteDeploymentRoute(gomock.Any(), gomock.Any(), "test-deployment", "default").Return(notFoundErr)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			tt.setupMocks(mockGateway, mockProxy)

			actor := &CleanupActor{
				gateway:       mockGateway,
				proxyProvider: mockProxy,
				logger:        zap.NewNop(),
			}

			result, err := actor.Run(context.Background(), tt.deployment, tt.condition())

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, result.Message)
			}
		})
	}
}
