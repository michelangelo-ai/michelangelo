package creation

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
	backendsmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestClusterWorkloadsActor_Retrieve(t *testing.T) {
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
			name: "all clusters ready",
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
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "cluster not ready",
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
						ClusterState: v2pb.CLUSTER_STATE_CREATING,
					}, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:        "ClusterNotReady",
			expectedReasonContains: "is in state",
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
			expectedMessage:        "ClusterCheckFailed",
			expectedReasonContains: "Failed to check cluster test-cluster status",
			expectedErr:            false,
		},
		{
			name: "control plane cluster returns true when ready",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_ControlPlaneClusterDeployment{
							ControlPlaneClusterDeployment: &v2pb.ControlPlaneClusterDeployment{},
						},
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				// nil cluster target means control plane cluster
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), "test-server", "test-namespace", nil).
					Return(&backends.ServerStatus{
						ClusterState: v2pb.CLUSTER_STATE_READY,
					}, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := NewClusterWorkloadsActor(mockBackend, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonClusterWorkloads",
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

func TestClusterWorkloadsActor_Run(t *testing.T) {
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
			name: "server creation succeeds",
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
					InitSpec: &v2pb.InitSpec{
						ResourceSpec: &v2pb.ResourceSpec{
							Cpu:    4,
							Memory: "8Gi",
							Gpu:    2,
						},
						NumInstances: 1,
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						"test-server",
						"test-namespace",
						backends.ResourceConstraints{
							Cpu:      4,
							Memory:   "8Gi",
							Gpu:      2,
							Replicas: 1,
						},
						testCluster,
					).
					DoAndReturn(func(ctx context.Context, name, namespace string, constraints backends.ResourceConstraints, cluster *v2pb.ClusterTarget) (*backends.ServerStatus, error) {
						assert.Equal(t, "test-server", name)
						assert.Equal(t, "test-namespace", namespace)
						assert.Equal(t, int32(4), constraints.Cpu)
						assert.Equal(t, "8Gi", constraints.Memory)
						assert.Equal(t, int32(2), constraints.Gpu)
						assert.Equal(t, int32(1), constraints.Replicas)
						assert.Equal(t, testCluster, cluster)
						return nil, nil
					})
			},
			expectedStatus:         apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:        "ClusterCreationInitiated",
			expectedReasonContains: "server creation initiated",
			expectedErr:            false,
		},
		{
			name: "server creation fails",
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
					InitSpec: &v2pb.InitSpec{
						ResourceSpec: &v2pb.ResourceSpec{
							Cpu:    4,
							Memory: "8Gi",
							Gpu:    2,
						},
						NumInstances: 1,
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						"test-server",
						"test-namespace",
						backends.ResourceConstraints{
							Cpu:      4,
							Memory:   "8Gi",
							Gpu:      2,
							Replicas: 1,
						},
						testCluster,
					).
					Return(nil, errors.New("insufficient resources"))
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "ClusterCreationFailed",
			expectedReasonContains: "Failed to create server",
			expectedErr:            false,
		},
		{
			name: "control plane cluster creation succeeds",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_ControlPlaneClusterDeployment{
							ControlPlaneClusterDeployment: &v2pb.ControlPlaneClusterDeployment{},
						},
					},
					InitSpec: &v2pb.InitSpec{
						ResourceSpec: &v2pb.ResourceSpec{
							Cpu:    4,
							Memory: "8Gi",
							Gpu:    2,
						},
						NumInstances: 1,
					},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						"test-server",
						"test-namespace",
						backends.ResourceConstraints{
							Cpu:      4,
							Memory:   "8Gi",
							Gpu:      2,
							Replicas: 1,
						},
						nil,
					).
					Return(nil, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:        "ClusterCreationInitiated",
			expectedReasonContains: "server creation initiated",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := NewClusterWorkloadsActor(mockBackend, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonClusterWorkloads",
			}

			result, err := actor.Run(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedMessage, result.Message)
				if tt.expectedReasonContains != "" {
					assert.Contains(t, result.Reason, tt.expectedReasonContains)
				}
			}
		})
	}
}
