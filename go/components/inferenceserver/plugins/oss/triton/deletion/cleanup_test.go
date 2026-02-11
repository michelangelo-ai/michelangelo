package deletion

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestCleanupActor_Retrieve(t *testing.T) {
	testCluster := &v2pb.ClusterTarget{ClusterId: "test-cluster"}

	tests := []struct {
		name                   string
		resource               *v2pb.InferenceServer
		setupMocks             func(*backendsmocks.MockBackend)
		expectedStatus         apipb.ConditionStatus
		expectedMessage        string
		expectedReasonContains string
		expectedErr            bool
	}{
		{
			name: "server still exists",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_RemoteClusterDeployment{
							RemoteClusterDeployment: &v2pb.RemoteClustersDeployment{
								ClusterTargets: []*v2pb.ClusterTarget{testCluster},
							},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(&backends.ServerStatus{
						ClusterState: v2pb.CLUSTER_STATE_READY,
					}, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "ServerNotDeleted",
			expectedReasonContains: "is not deleted",
			expectedErr:            false,
		},
		{
			name: "server deleted successfully",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_RemoteClusterDeployment{
							RemoteClusterDeployment: &v2pb.RemoteClustersDeployment{
								ClusterTargets: []*v2pb.ClusterTarget{testCluster},
							},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(&backends.ServerStatus{
						ClusterState: v2pb.CLUSTER_STATE_INVALID,
					}, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "error checking server status",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_RemoteClusterDeployment{
							RemoteClusterDeployment: &v2pb.RemoteClustersDeployment{
								ClusterTargets: []*v2pb.ClusterTarget{testCluster},
							},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(nil, errors.New("API error"))
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "CannotCheckServerStatus",
			expectedReasonContains: "Failed to check server status",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := NewCleanupActor(mockBackend, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonCleanup",
			}

			result, err := actor.Retrieve(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				if tt.expectedMessage != "" {
					assert.Equal(t, tt.expectedMessage, result.Message)
				}
				if tt.expectedReasonContains != "" {
					assert.Contains(t, result.Reason, tt.expectedReasonContains)
				}
			}
		})
	}
}

func TestCleanupActor_Run(t *testing.T) {
	testCluster := &v2pb.ClusterTarget{ClusterId: "test-cluster"}

	tests := []struct {
		name                   string
		resource               *v2pb.InferenceServer
		setupMocks             func(*backendsmocks.MockBackend)
		expectedStatus         apipb.ConditionStatus
		expectedMessage        string
		expectedReasonContains string
		expectedErr            bool
	}{
		{
			name: "successful cleanup",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_RemoteClusterDeployment{
							RemoteClusterDeployment: &v2pb.RemoteClustersDeployment{
								ClusterTargets: []*v2pb.ClusterTarget{testCluster},
							},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "deletion fails",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_RemoteClusterDeployment{
							RemoteClusterDeployment: &v2pb.RemoteClustersDeployment{
								ClusterTargets: []*v2pb.ClusterTarget{testCluster},
							},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(errors.New("failed to delete deployment"))
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "ServerCleanupFailed",
			expectedReasonContains: "failed to cleanup inference server",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := NewCleanupActor(mockBackend, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonCleanup",
			}

			result, err := actor.Run(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedStatus, result.Status)
				if tt.expectedMessage != "" {
					assert.Equal(t, tt.expectedMessage, result.Message)
				}
				if tt.expectedReasonContains != "" {
					assert.Contains(t, result.Reason, tt.expectedReasonContains)
				}
			}
		})
	}
}
