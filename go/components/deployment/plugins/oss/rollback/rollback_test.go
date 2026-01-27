package rollback

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
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRollbackActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *apipb.Condition
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "no candidate revision returns true",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: nil,
				},
			},
			condition:       func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "no metadata returns RollbackNotStarted",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition:       func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "RollbackNotStarted",
		},
		{
			name: "all clusters rolled back returns true",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", State: common.ClusterStateRolledBack},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
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
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "RollbackPending",
		},
		{
			name: "rollback in progress and model still exists",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStateRollbackInProgress},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelExists(
					gomock.Any(), gomock.Any(), "failed-model", "test-server", "default", gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "RollbackInProgress",
		},
		{
			name: "rollback in progress check fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStateRollbackInProgress},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().CheckModelExists(
					gomock.Any(), gomock.Any(), "failed-model", "test-server", "default", gomock.Any(), v2pb.BACKEND_TYPE_TRITON,
				).Return(false, errors.New("api error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "RollbackStatusCheckFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &RollbackActor{
				logger:  zap.NewNop(),
				gateway: mockGateway,
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

func TestRollbackActor_Run(t *testing.T) {
	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		condition       func() *apipb.Condition
		setupMocks      func(*gatewaysmocks.MockGateway)
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
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType: v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*v2pb.ClusterTarget{
							{ClusterId: "cluster-1", Config: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{Host: "host1"}}},
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "MetadataInitialized",
		},
		{
			name: "get target info fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition { return &apipb.Condition{} },
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(nil, errors.New("not found"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "GetTargetInfoFailed",
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
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "failed-model", "test-server", "default", gomock.Any(),
				).Return(nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "RollbackStarted",
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
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", Host: "host1", Port: "6443", State: common.ClusterStatePending},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().UnloadModel(
					gomock.Any(), gomock.Any(), "failed-model", "test-server", "default", gomock.Any(),
				).Return(errors.New("unload failed"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "RollbackFailed",
		},
		{
			name: "all clusters rolled back returns true",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", State: common.ClusterStateRolledBack},
					},
					CurrentIndex: 1,
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "rollback in progress returns unknown",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
				Status: v2pb.DeploymentStatus{
					CandidateRevision: &apipb.ResourceIdentifier{Name: "failed-model"},
				},
			},
			condition: func() *apipb.Condition {
				cond := &apipb.Condition{}
				_ = common.SetClusterMetadata(cond, &common.ClusterMetadata{
					BackendType: "BACKEND_TYPE_TRITON",
					Clusters: []common.ClusterEntry{
						{ClusterID: "cluster-1", State: common.ClusterStateRollbackInProgress},
					},
					CurrentIndex: 0,
				})
				return cond
			},
			setupMocks:      func(gw *gatewaysmocks.MockGateway) {},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage: "RollbackInProgress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &RollbackActor{
				logger:  zap.NewNop(),
				gateway: mockGateway,
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
