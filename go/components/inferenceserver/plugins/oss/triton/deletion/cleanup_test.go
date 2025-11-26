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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	configmapmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap/configmapmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	gatewaysmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	proxymocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy/proxymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestCleanupActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "infrastructure exists, cleanup not completed",
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
					Return(&gateways.GetInfrastructureStatusResponse{}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "CleanupInProgress",
			expectedMessage: "Infrastructure cleanup in progress",
			expectedErr:     false,
		},
		{
			name: "infrastructure does not exist, cleanup completed",
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
					Return(nil, errors.New("infrastructure not found"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "CleanupCompleted",
			expectedMessage: "Infrastructure cleanup completed",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockConfigMapProvider := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			mockProxyProvider := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockGateway)

			actor := NewCleanupActor(mockGateway, mockConfigMapProvider, mockProxyProvider, zap.NewNop())

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
		setupMocks              func(*gatewaysmocks.MockGateway, *configmapmocks.MockModelConfigMapProvider, *proxymocks.MockProxyProvider)
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "successful cleanup, all resources deleted",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway, mockConfigMap *configmapmocks.MockModelConfigMapProvider, mockProxy *proxymocks.MockProxyProvider) {
				// ConfigMap deletion succeeds
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(
						gomock.Any(),
						configmap.DeleteModelConfigMapRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// HTTPRoute deletion succeeds
				mockProxy.EXPECT().
					DeleteInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.DeleteInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// Infrastructure deletion succeeds
				mockGateway.EXPECT().
					DeleteInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gateways.DeleteInfrastructureRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully",
			expectedErr:             false,
		},
		{
			name: "configmap deletion fails, cleanup continues",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway, mockConfigMap *configmapmocks.MockModelConfigMapProvider, mockProxy *proxymocks.MockProxyProvider) {
				// ConfigMap deletion fails
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(
						gomock.Any(),
						configmap.DeleteModelConfigMapRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(errors.New("configmap not found"))

				// HTTPRoute deletion succeeds
				mockProxy.EXPECT().
					DeleteInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.DeleteInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// Infrastructure deletion succeeds
				mockGateway.EXPECT().
					DeleteInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gateways.DeleteInfrastructureRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully",
			expectedErr:             false,
		},
		{
			name: "httproute deletion fails, cleanup continues",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway, mockConfigMap *configmapmocks.MockModelConfigMapProvider, mockProxy *proxymocks.MockProxyProvider) {
				// ConfigMap deletion succeeds
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(
						gomock.Any(),
						configmap.DeleteModelConfigMapRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// HTTPRoute deletion fails
				mockProxy.EXPECT().
					DeleteInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.DeleteInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(errors.New("httproute not found"))

				// Infrastructure deletion succeeds
				mockGateway.EXPECT().
					DeleteInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gateways.DeleteInfrastructureRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully",
			expectedErr:             false,
		},
		{
			name: "infrastructure deletion fails, returns error",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway, mockConfigMap *configmapmocks.MockModelConfigMapProvider, mockProxy *proxymocks.MockProxyProvider) {
				// ConfigMap deletion succeeds
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(
						gomock.Any(),
						configmap.DeleteModelConfigMapRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// HTTPRoute deletion succeeds
				mockProxy.EXPECT().
					DeleteInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.DeleteInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil)

				// Infrastructure deletion fails
				mockGateway.EXPECT().
					DeleteInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gateways.DeleteInfrastructureRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(errors.New("failed to delete deployment"))
			},
			expectedStatus:          apipb.CONDITION_STATUS_FALSE,
			expectedReason:          "InfrastructureCleanupFailed",
			expectedMessageContains: "Failed to cleanup infrastructure",
			expectedErr:             true,
		},
		{
			name: "both configmap and httproute deletion fail but infrastructure cleanup succeeds",
			setupMocks: func(mockGateway *gatewaysmocks.MockGateway, mockConfigMap *configmapmocks.MockModelConfigMapProvider, mockProxy *proxymocks.MockProxyProvider) {
				// ConfigMap deletion fails
				mockConfigMap.EXPECT().
					DeleteModelConfigMap(
						gomock.Any(),
						configmap.DeleteModelConfigMapRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(errors.New("configmap error"))

				// HTTPRoute deletion fails
				mockProxy.EXPECT().
					DeleteInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.DeleteInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(errors.New("httproute error"))

				// Infrastructure deletion succeeds
				mockGateway.EXPECT().
					DeleteInfrastructure(
						gomock.Any(),
						gomock.Any(),
						gateways.DeleteInfrastructureRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							BackendType:     v2pb.BACKEND_TYPE_TRITON,
						},
					).
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "CleanupInitiated",
			expectedMessageContains: "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully",
			expectedErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockConfigMapProvider := configmapmocks.NewMockModelConfigMapProvider(ctrl)
			mockProxyProvider := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockGateway, mockConfigMapProvider, mockProxyProvider)

			actor := NewCleanupActor(mockGateway, mockConfigMapProvider, mockProxyProvider, zap.NewNop())

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
				// When there's an error, the function returns nil but modifies the condition parameter
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			// Check condition status and reason (the condition parameter is modified in-place)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.Contains(t, condition.Message, tt.expectedMessageContains)
		})
	}
}
