package creation

import (
	"context"
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

// createValidationTestRegistry creates a registry with the mock backend registered for Triton.
func createValidationTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	return registry
}

func TestValidationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		backendType     v2pb.BackendType
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name:            "valid triton backend type",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "",
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name:            "invalid backend type - llm-d",
			backendType:     v2pb.BACKEND_TYPE_LLM_D,
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidBackendType",
			expectedReason:  "unsupported backend type: BACKEND_TYPE_LLM_D",
			expectedErr:     false,
		},
		{
			name:            "invalid backend type",
			backendType:     v2pb.BACKEND_TYPE_INVALID,
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidBackendType",
			expectedReason:  "unsupported backend type: BACKEND_TYPE_INVALID",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			registry := createValidationTestRegistry(mockBackend)
			// No expectations set, backend should not be called

			actor := NewValidationActor(registry, zap.NewNop())

			resource := &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: tt.backendType,
				},
			}

			condition := &apipb.Condition{
				Type: "TritonValidation",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonValidation", result.Type)
			}
		})
	}
}

func TestValidationActor_Run(t *testing.T) {
	// Run() simply returns the input condition as-is (no changes).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := backendsmocks.NewMockBackend(ctrl)
	registry := createValidationTestRegistry(mockBackend)
	// No expectations set, backend should not be called

	actor := NewValidationActor(registry, zap.NewNop())

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
		Type:    "TritonValidation",
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "TestReason",
		Message: "TestMessage",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, result.Status)
	assert.Equal(t, "TestReason", result.Reason)
	assert.Equal(t, "TestMessage", result.Message)
	assert.Equal(t, "TritonValidation", result.Type)
}
