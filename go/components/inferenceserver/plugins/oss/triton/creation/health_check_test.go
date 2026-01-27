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

	backendsmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestHealthCheckActor_Retrieve(t *testing.T) {
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
			name: "server is healthy",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType:    v2pb.BACKEND_TYPE_TRITON,
					ClusterTargets: []*v2pb.ClusterTarget{testCluster},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(true, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_TRUE,
			expectedMessage:        "",
			expectedReasonContains: "",
			expectedErr:            false,
		},
		{
			name: "server is not healthy",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType:    v2pb.BACKEND_TYPE_TRITON,
					ClusterTargets: []*v2pb.ClusterTarget{testCluster},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(false, nil)
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "HealthCheckFailed",
			expectedReasonContains: "Server is not healthy in cluster",
			expectedErr:            false,
		},
		{
			name: "health check returns error",
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType:    v2pb.BACKEND_TYPE_TRITON,
					ClusterTargets: []*v2pb.ClusterTarget{testCluster},
				},
			},
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(gomock.Any(), "test-server", "test-namespace", testCluster).
					Return(false, errors.New("connection timeout"))
			},
			expectedStatus:         apipb.CONDITION_STATUS_FALSE,
			expectedMessage:        "HealthCheckFailed",
			expectedReasonContains: "Health check error: connection timeout",
			expectedErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			tt.setupMocks(mockBackend)

			actor := NewHealthCheckActor(mockBackend, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonHealthCheck",
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

func TestHealthCheckActor_Run(t *testing.T) {
	// Run() returns the condition unchanged.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := backendsmocks.NewMockBackend(ctrl)
	// No expectations set, backend should not be called

	actor := NewHealthCheckActor(mockBackend, zap.NewNop())

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
		Type:   "TritonHealthCheck",
		Status: apipb.CONDITION_STATUS_UNKNOWN,
		Reason: "TestReason",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Run returns the same condition unchanged
	assert.Equal(t, condition, result)
}
