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

// createHealthCheckTestRegistry creates a registry with the mock backend registered for supported types.
func createHealthCheckTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	registry.Register(v2pb.BACKEND_TYPE_DYNAMO, mockBackend)
	return registry
}

func TestHealthCheckActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*backendsmocks.MockBackend)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "server is healthy",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "",
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name: "server is not healthy",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HealthCheckFailed",
			expectedReason:  "Server is not healthy",
			expectedErr:     false,
		},
		{
			name: "health check returns error",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(false, errors.New("connection timeout"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HealthCheckFailed",
			expectedReason:  "Health check error: connection timeout",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			registry := createHealthCheckTestRegistry(mockBackend)

			tt.setupMocks(mockBackend)

			actor := NewHealthCheckActor(nil, registry, zap.NewNop())

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
				Type: "TritonHealthCheck",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonHealthCheck", result.Type)
			}
		})
	}
}

func TestHealthCheckActor_Run(t *testing.T) {
	// Run() simply returns the input condition as-is (no changes).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := backendsmocks.NewMockBackend(ctrl)
	registry := createHealthCheckTestRegistry(mockBackend)
	// No expectations set, backend should not be called

	actor := NewHealthCheckActor(nil, registry, zap.NewNop())

	resource := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
		Spec: v2pb.InferenceServerSpec{
			BackendType: v2pb.BACKEND_TYPE_TRITON,
		},
	}

	// Provide an input condition with specific values
	condition := &apipb.Condition{
		Type:    "TritonHealthCheck",
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "TestReason",
		Message: "TestMessage",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Run() returns the input condition as-is
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, result.Status)
	assert.Equal(t, "TestReason", result.Reason)
	assert.Equal(t, "TestMessage", result.Message)
	assert.Equal(t, "TritonHealthCheck", result.Type)
}
