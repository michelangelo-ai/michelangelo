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
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

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
						"test-server",
						"test-namespace",
					).
					Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "HealthCheckSucceeded",
			expectedMessage: "Server is healthy",
			expectedErr:     false,
		},
		{
			name: "server is not healthy",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "HealthCheckFailed",
			expectedMessage: "Server is not healthy",
			expectedErr:     false,
		},
		{
			name: "health check returns error",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					IsHealthy(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(false, errors.New("connection timeout"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "HealthCheckFailed",
			expectedMessage: "Health check error: connection timeout",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)

			tt.setupMocks(mockBackend)

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
	// Run() always returns a simple false condition since there's nothing
	// it can do differently from Retrieve(). It doesn't call the backend.
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
		Type: "TritonHealthCheck",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, result.Status)
	assert.Equal(t, "HealthCheckFailed", result.Reason)
	assert.Equal(t, "Server is not healthy", result.Message)
	assert.Equal(t, "TritonHealthCheck", result.Type)
}
