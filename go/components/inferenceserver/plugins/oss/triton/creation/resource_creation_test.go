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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	gatewaysmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestResourceCreationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "infrastructure is ready and serving",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					GetInfrastructureStatus(
						gomock.Any(),
						gomock.Any(),
						gateways.GetInfrastructureStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(&gateways.GetInfrastructureStatusResponse{
						Status: gateways.InfrastructureStatus{
							State: v2pb.INFERENCE_SERVER_STATE_SERVING,
							Ready: true,
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "InfrastructureReady",
			expectedMessage: "Infrastructure is ready",
			expectedErr:     false,
		},
		{
			name: "infrastructure is creating",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					GetInfrastructureStatus(
						gomock.Any(),
						gomock.Any(),
						gateways.GetInfrastructureStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(&gateways.GetInfrastructureStatusResponse{
						Status: gateways.InfrastructureStatus{
							State: v2pb.INFERENCE_SERVER_STATE_CREATING,
							Ready: false,
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "InfrastructureNotFound",
			expectedMessage: "Infrastructure needs to be created",
			expectedErr:     false,
		},
		{
			name: "error checking infrastructure status",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					GetInfrastructureStatus(
						gomock.Any(),
						gomock.Any(),
						gateways.GetInfrastructureStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(nil, errors.New("API error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "InfrastructureCheckFailed",
			expectedMessage: "Failed to check infrastructure status: API error",
			expectedErr:     false,
		},
		{
			name: "infrastructure in other state",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					GetInfrastructureStatus(
						gomock.Any(),
						gomock.Any(),
						gateways.GetInfrastructureStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(&gateways.GetInfrastructureStatusResponse{
						Status: gateways.InfrastructureStatus{
							State: v2pb.INFERENCE_SERVER_STATE_FAILED,
							Ready: false,
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "InfrastructureCreating",
			expectedMessage: "Infrastructure is being created",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)

			tt.setupMocks(mockGateway)

			actor := NewResourceCreationActor(mockGateway, zap.NewNop())

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
		setupMocks              func(*gatewaysmocks.MockGateway)
		resource                *v2pb.InferenceServer
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "infrastructure creation succeeds",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					CreateInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					DoAndReturn(func(ctx context.Context, logger *zap.Logger, req gateways.CreateInfrastructureRequest) (*gateways.CreateInfrastructureResponse, error) {
						assert.Equal(t, "test-server", req.InferenceServer.Name)
						assert.Equal(t, "test-namespace", req.Namespace)
						assert.Equal(t, v2pb.BACKEND_TYPE_TRITON, req.BackendType)
						assert.Equal(t, "4", req.Resources.CPU)
						assert.Equal(t, "8Gi", req.Resources.Memory)
						assert.Equal(t, int32(2), req.Resources.GPU)
						assert.Equal(t, int32(1), req.Resources.Replicas)
						return &gateways.CreateInfrastructureResponse{
							State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
							Message: "Infrastructure creation initiated",
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
					},
				},
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "InfrastructureCreationInitiated",
			expectedMessageContains: "Infrastructure creation initiated successfully",
			expectedErr:             false,
		},
		{
			name: "infrastructure creation fails",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway) {
				mockGateway.EXPECT().
					CreateInfrastructure(
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
			expectedReason:          "InfrastructureCreationFailed",
			expectedMessageContains: "Failed to create infrastructure: insufficient resources",
			expectedErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)

			tt.setupMocks(mockGateway)

			actor := NewResourceCreationActor(mockGateway, zap.NewNop())

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
