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

// createTestRegistry creates a registry with the mock backend registered for supported types.
func createTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	registry.Register(v2pb.BACKEND_TYPE_DYNAMO, mockBackend)
	return registry
}

func TestBackendProvisioningActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*backendsmocks.MockBackend)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "Backend is ready and serving",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_SERVING,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "",
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name: "server is creating",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_CREATING,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "BackendProvisioningFailed",
			expectedReason:  "Backend state is not serving: INFERENCE_SERVER_STATE_CREATING",
			expectedErr:     false,
		},
		{
			name: "error checking server status",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(nil, errors.New("API error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "BackendProvisioningCheckFailed",
			expectedReason:  "Failed to check backend status: API error",
			expectedErr:     false,
		},
		{
			name: "server in other state",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_FAILED,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "BackendProvisioningFailed",
			expectedReason:  "Backend state is not serving: INFERENCE_SERVER_STATE_FAILED",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			registry := createTestRegistry(mockBackend)

			tt.setupMocks(mockBackend)

			actor := NewBackendProvisionActor(nil, registry, zap.NewNop())

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
				Type: "TritonResourceCreation",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonResourceCreation", result.Type)
			}
		})
	}
}

func TestResourceCreationActor_Run(t *testing.T) {
	tests := []struct {
		name                    string
		setupMocks              func(*testing.T, *backendsmocks.MockBackend)
		resource                *v2pb.InferenceServer
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "server creation succeeds",
			setupMocks: func(t *testing.T, mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					DoAndReturn(func(ctx context.Context, logger *zap.Logger, kubeClient interface{}, inferenceServer *v2pb.InferenceServer) (*backends.ServerStatus, error) {
						assert.Equal(t, "test-server", inferenceServer.Name)
						assert.Equal(t, "test-namespace", inferenceServer.Namespace)
						assert.Equal(t, v2pb.BACKEND_TYPE_TRITON, inferenceServer.Spec.BackendType)
						assert.NotNil(t, inferenceServer.Spec.InitSpec)
						assert.NotNil(t, inferenceServer.Spec.InitSpec.ResourceSpec)
						assert.Equal(t, int32(4), inferenceServer.Spec.InitSpec.ResourceSpec.Cpu)
						assert.Equal(t, "8Gi", inferenceServer.Spec.InitSpec.ResourceSpec.Memory)
						assert.Equal(t, int32(2), inferenceServer.Spec.InitSpec.ResourceSpec.Gpu)
						assert.Equal(t, int32(1), inferenceServer.Spec.InitSpec.NumInstances)
						return &backends.ServerStatus{
							State: v2pb.INFERENCE_SERVER_STATE_CREATING,
						}, nil
					})
			},
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
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
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "",
			expectedMessageContains: "",
			expectedErr:             false,
		},
		{
			name: "server creation fails",
			setupMocks: func(t *testing.T, mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					Return(nil, errors.New("insufficient resources"))
			},
			resource: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					InitSpec: &v2pb.InitSpec{
						ResourceSpec: &v2pb.ResourceSpec{
							Cpu:    4,
							Memory: "8Gi",
							Gpu:    2,
						},
					},
				},
			},
			expectedStatus:          apipb.CONDITION_STATUS_FALSE,
			expectedReason:          "Failed to provision backend: insufficient resources",
			expectedMessageContains: "BackendProvisionFailed",
			expectedErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			registry := createTestRegistry(mockBackend)

			tt.setupMocks(t, mockBackend)

			actor := NewBackendProvisionActor(nil, registry, zap.NewNop())

			condition := &apipb.Condition{
				Type: "TritonResourceCreation",
			}

			result, err := actor.Run(context.Background(), tt.resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedReason, result.Reason)
			assert.Contains(t, result.Message, tt.expectedMessageContains)
		})
	}
}
