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

	gatewaysmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	proxymocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy/proxymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestProxyConfigurationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*proxymocks.MockProxyProvider)
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name: "proxy is configured",
			setupMocks: func(mockProxy *proxymocks.MockProxyProvider) {
				mockProxy.EXPECT().
					GetProxyStatus(
						gomock.Any(),
						gomock.Any(),
						proxy.GetProxyStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(&proxy.GetProxyStatusResponse{
						Status: proxy.ProxyStatus{
							Configured: true,
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "ProxyConfigured",
			expectedMessage: "Proxy is configured and ready",
			expectedErr:     false,
		},
		{
			name: "proxy is not configured",
			setupMocks: func(mockProxy *proxymocks.MockProxyProvider) {
				mockProxy.EXPECT().
					GetProxyStatus(
						gomock.Any(),
						gomock.Any(),
						proxy.GetProxyStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(&proxy.GetProxyStatusResponse{
						Status: proxy.ProxyStatus{
							Configured: false,
						},
					}, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "ProxyNotConfigured",
			expectedMessage: "Proxy is not configured",
			expectedErr:     false,
		},
		{
			name: "error checking proxy status",
			setupMocks: func(mockProxy *proxymocks.MockProxyProvider) {
				mockProxy.EXPECT().
					GetProxyStatus(
						gomock.Any(),
						gomock.Any(),
						proxy.GetProxyStatusRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
						},
					).
					Return(nil, errors.New("connection timeout"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedReason:  "ProxyNotConfigured",
			expectedMessage: "Failed to check proxy status: connection timeout",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockProxy)

			actor := NewProxyConfigurationActor(mockGateway, mockProxy, zap.NewNop())

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
				Type: "TritonProxyConfiguration",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonProxyConfiguration", result.Type)
			}
		})
	}
}

func TestProxyConfigurationActor_Run(t *testing.T) {
	tests := []struct {
		name                    string
		setupMocks              func(*proxymocks.MockProxyProvider)
		expectedStatus          apipb.ConditionStatus
		expectedReason          string
		expectedMessageContains string
		expectedErr             bool
	}{
		{
			name: "proxy configuration succeeds",
			setupMocks: func(mockProxy *proxymocks.MockProxyProvider) {
				mockProxy.EXPECT().
					EnsureInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.EnsureInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							ModelName:       "test-server",
						},
					).
					Return(nil)
			},
			expectedStatus:          apipb.CONDITION_STATUS_TRUE,
			expectedReason:          "ProxyConfigured",
			expectedMessageContains: "Proxy configured successfully",
			expectedErr:             false,
		},
		{
			name: "proxy configuration fails",
			setupMocks: func(mockProxy *proxymocks.MockProxyProvider) {
				mockProxy.EXPECT().
					EnsureInferenceServerRoute(
						gomock.Any(),
						gomock.Any(),
						proxy.EnsureInferenceServerRouteRequest{
							InferenceServer: "test-server",
							Namespace:       "test-namespace",
							ModelName:       "test-server",
						},
					).
					Return(errors.New("route creation failed"))
			},
			expectedStatus:          apipb.CONDITION_STATUS_FALSE,
			expectedReason:          "ProxyConfigurationFailed",
			expectedMessageContains: "Failed to configure proxy: route creation failed",
			expectedErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			mockProxy := proxymocks.NewMockProxyProvider(ctrl)

			tt.setupMocks(mockProxy)

			actor := NewProxyConfigurationActor(mockGateway, mockProxy, zap.NewNop())

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
				Type: "TritonProxyConfiguration",
			}

			result, err := actor.Run(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
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
