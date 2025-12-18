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
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestResourceCreationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*backendsmocks.MockBackend)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "Inference server is ready and serving",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_SERVING,
						Ready: true,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "ServerReady",
			expectedMessage: "Server is ready",
			expectedErr:     false,
		},
		{
			name: "server is creating",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_CREATING,
						Ready: false,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "ServerNotFound",
			expectedMessage: "Server needs to be created",
			expectedErr:     false,
		},
		{
			name: "error checking server status",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(nil, errors.New("API error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "ServerCheckFailed",
			expectedMessage: "Failed to check server status: API error",
			expectedErr:     false,
		},
		{
			name: "server in other state",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					GetServerStatus(
						gomock.Any(),
						gomock.Any(),
						"test-server",
						"test-namespace",
					).
					Return(&backends.ServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_FAILED,
						Ready: false,
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "ServerCreating",
			expectedMessage: "Server is being created",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)

			tt.setupMocks(mockBackend)

			actor := NewResourceCreationActor(mockBackend, zap.NewNop())

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
		setupMocks              func(*backendsmocks.MockBackend)
		resource                *v2pb.InferenceServer
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "server creation succeeds",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					DoAndReturn(func(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (*backends.ServerStatus, error) {
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
							State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
							Message: "Server creation initiated",
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
			expectedReason:          "ServerCreationInitiated",
			expectedMessageContains: "Server creation initiated successfully",
			expectedErr:             false,
		},
		{
			name: "server creation fails",
			setupMocks: func(mockBackend *backendsmocks.MockBackend) {
				mockBackend.EXPECT().
					CreateServer(
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
			expectedReason:          "ServerCreationFailed",
			expectedMessageContains: "Failed to create server: insufficient resources",
			expectedErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)

			tt.setupMocks(mockBackend)

			actor := NewResourceCreationActor(mockBackend, zap.NewNop())

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
