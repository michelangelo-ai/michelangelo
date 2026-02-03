package creation

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestValidationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name                   string
		resource               *v2pb.InferenceServer
		expectedStatus         apipb.ConditionStatus
		expectedMessage        string
		expectedReasonContains string
		expectedErr            bool
	}{
		{
			name: "valid triton with control plane cluster deployment",
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
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "valid triton with nil deployment strategy defaults to control plane",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "valid triton with remote cluster",
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
								ClusterTargets: []*v2pb.ClusterTarget{
									{
										ClusterId: "remote-cluster",
										Config: &v2pb.ClusterTarget_Kubernetes{
											Kubernetes: &v2pb.ConnectionSpec{
												Host:      "https://api.remote.cluster",
												Port:      "6443",
												TokenTag:  "token-secret",
												CaDataTag: "ca-secret",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "invalid backend type",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_LLM_D,
					DeploymentStrategy: &v2pb.InferenceServerDeploymentStrategy{
						Strategy: &v2pb.InferenceServerDeploymentStrategy_ControlPlaneClusterDeployment{
							ControlPlaneClusterDeployment: &v2pb.ControlPlaneClusterDeployment{},
						},
					},
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "InvalidBackendType",
			expectedReasonContains: "invalid backend type for Triton plugin",
			expectedErr:            false,
		},
		{
			name: "no cluster targets in remote deployment",
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
								ClusterTargets: []*v2pb.ClusterTarget{},
							},
						},
					},
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "InvalidClusterTargets",
			expectedReasonContains: "at least one cluster target is required",
			expectedErr:            false,
		},
		{
			name: "remote cluster missing kubernetes config",
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
								ClusterTargets: []*v2pb.ClusterTarget{
									{ClusterId: "remote-cluster"}, // no kubernetes config
								},
							},
						},
					},
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "InvalidClusterTargets",
			expectedReasonContains: "kubernetes connection config is required",
			expectedErr:            false,
		},
		{
			name: "remote cluster missing host",
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
								ClusterTargets: []*v2pb.ClusterTarget{
									{
										ClusterId: "remote-cluster",
										Config: &v2pb.ClusterTarget_Kubernetes{
											Kubernetes: &v2pb.ConnectionSpec{
												Port:      "6443",
												TokenTag:  "token-secret",
												CaDataTag: "ca-secret",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "InvalidClusterTargets",
			expectedReasonContains: "host is required",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			actor := NewValidationActor(zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonValidation",
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

func TestValidationActor_Run(t *testing.T) {
	// Run() returns the condition unchanged - it's a no-op for validation.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	actor := NewValidationActor(zap.NewNop())

	resource := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
		Spec: v2pb.InferenceServerSpec{
			BackendType: v2pb.BACKEND_TYPE_TRITON,
		},
	}

	condition := &apipb.Condition{
		Type:   "TritonValidation",
		Status: apipb.CONDITION_STATUS_FALSE,
		Reason: "TestReason",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Run returns the same condition unchanged
	assert.Equal(t, condition, result)
}
