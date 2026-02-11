package strategies

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestModelCleanupActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *api.Condition
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  api.ConditionStatus
		expectedMessage string
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
			condition:       func() *api.Condition { return &api.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_TRUE,
			expectedMessage: "",
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
			condition:       func() *api.Condition { return &api.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "cleanup not started when no metadata",
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
			condition:       func() *api.Condition { return &api.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_FALSE,
			expectedMessage: "CleanupNotStarted",
		},
		{
			name: "cleanup pending for cluster in PENDING state",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 0,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStatePending},
					},
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_FALSE,
			expectedMessage: "CleanupPending",
		},
		{
			name: "cleanup in progress: old model still loaded",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 0,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStateCleanupInProgress},
					},
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
			},
			expectedStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "CleanupInProgress",
		},
		{
			name: "all clusters cleaned",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 1,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStateCleaned},
					},
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_TRUE,
			expectedMessage: "",
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

func TestModelCleanupActor_Run(t *testing.T) {
	testCluster := &gateways.TargetClusterConnection{
		ClusterId: "test-cluster",
		Host:      "host1",
	}

	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *api.Condition
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  api.ConditionStatus
		expectedMessage string
	}{
		{
			name: "initialize metadata when none exists",
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
			condition: func() *api.Condition { return &api.Condition{} },
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
			},
			expectedStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "MetadataInitialized",
		},
		{
			name: "GetDeploymentTargetInfo fails",
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
			condition: func() *api.Condition { return &api.Condition{} },
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(nil, errors.New("not found"))
			},
			expectedStatus:  api.CONDITION_STATUS_FALSE,
			expectedMessage: "GetTargetInfoFailed",
		},
		{
			name: "unload model from pending cluster",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 0,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStatePending},
					},
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", gomock.Any(),
				).Return(nil)
			},
			expectedStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "CleanupStarted",
		},
		{
			name: "unload model fails",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 0,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStatePending},
					},
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", gomock.Any(),
				).Return(errors.New("unload error"))
			},
			expectedStatus:  api.CONDITION_STATUS_FALSE,
			expectedMessage: "ModelUnloadingFailed",
		},
		{
			name: "cleanup in progress - wait for Retrieve",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 0,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStateCleanupInProgress},
					},
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "CleanupInProgress",
		},
		{
			name: "all clusters already cleaned",
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
			condition: func() *api.Condition {
				cond := &api.Condition{}
				common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType:  v2pb.BACKEND_TYPE_TRITON.String(),
					CurrentIndex: 1,
					Clusters: []common.ClusterEntry{
						{ClusterId: "test-cluster", Host: "host1", State: common.ClusterStateCleaned},
					},
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  api.CONDITION_STATUS_TRUE,
			expectedMessage: "",
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
