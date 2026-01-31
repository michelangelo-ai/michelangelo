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
	configmapmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap/configmapmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestCleanupActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*backendsmocks.MockBackend)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "inference server exists, cleanup not completed",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(&backends.ServerStatus{}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "CleanupInProgress",
			expectedMessage: "Inference server cleanup in progress",
			expectedErr:     false,
		},
		{
			name: "inference server does not exist, cleanup completed",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil, errors.New("inference server not found"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "CleanupCompleted",
			expectedMessage: "Inference server cleanup completed",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			mockConfigMapProvider := configmapmocks.NewMockModelConfigMapProvider(ctrl)

			tt.setupMocks(mockBackend)

			actor := NewCleanupActor(mockBackend, mockConfigMapProvider, zap.NewNop())

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
				Type: "TritonCleanup",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonCleanup", result.Type)
			}
		})
	}
}

func TestCleanupActor_Run(t *testing.T) {
	tests := []struct {
		name                    string
		setupMocks              func(*backendsmocks.MockBackend, *configmapmocks.MockModelConfigMapProvider)
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "successful cleanup, all resources deleted",
			setupMocks: func(mockBackend *backendsmocks.MockBackend, mockConfigMap *configmapmocks.MockModelConfigMapProvider) {
				// ConfigMap deletion succeeds
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(gomock.Any(), "test-server", "test-namespace").
					Return(nil)

				// Inference server deletion succeeds
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Inference server, model ConfigMap cleanup initiated successfully",
			expectedErr:             false,
		},
		{
			name: "configmap deletion fails, cleanup continues",
			setupMocks: func(mockBackend *backendsmocks.MockBackend, mockConfigMap *configmapmocks.MockModelConfigMapProvider) {
				// ConfigMap deletion fails
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(gomock.Any(), "test-server", "test-namespace").
					Return(errors.New("configmap not found"))

				// Inference server deletion succeeds
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Inference server, model ConfigMap cleanup initiated successfully",
			expectedErr:             false,
		},
		{
			name: "inference server deletion fails, returns error",
			setupMocks: func(mockBackend *backendsmocks.MockBackend, mockConfigMap *configmapmocks.MockModelConfigMapProvider) {
				// ConfigMap deletion succeeds
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(gomock.Any(), "test-server", "test-namespace").
					Return(nil)

				// Inference server deletion fails
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(errors.New("failed to delete deployment"))
			},
			expectedStatus:          apipb.CONDITION_STATUS_FALSE,
			expectedReason:          "ServerCleanupFailed",
			expectedMessageContains: "Failed to cleanup inference server",
			expectedErr:             true,
		},
		{
			name: "configmap deletion fails but inference server cleanup succeeds",
			setupMocks: func(mockBackend *backendsmocks.MockBackend, mockConfigMap *configmapmocks.MockModelConfigMapProvider) {
				// ConfigMap deletion fails
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(gomock.Any(), "test-server", "test-namespace").
					Return(errors.New("configmap error"))

				// Inference server deletion succeeds
				mockBackend.EXPECT().
					DeleteServer(gomock.Any(), gomock.Any(), "test-server", "test-namespace").
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Inference server, model ConfigMap cleanup initiated successfully",
			expectedErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			mockConfigMapProvider := configmapmocks.NewMockModelConfigMapProvider(ctrl)

			tt.setupMocks(mockBackend, mockConfigMapProvider)

			actor := NewCleanupActor(mockBackend, mockConfigMapProvider, zap.NewNop())

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
				Type: "TritonCleanup",
			}

			result, err := actor.Run(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			// Check returned condition status and reason
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedReason, result.Reason)
			assert.Contains(t, result.Message, tt.expectedMessageContains)
		})
	}
}
