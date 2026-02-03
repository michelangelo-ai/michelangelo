package common

import (
	"context"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy/proxymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestTrafficRoutingRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*proxymocks.MockProxyProvider, *gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "traffic routing configured successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetControlPlaneServiceName(gomock.Any(), gomock.Any(), "test-server", "default").Return("test-server-svc", nil)
				pp.EXPECT().CheckDeploymentRouteStatus(
					gomock.Any(), gomock.Any(), "test-deployment", "default", "test-server", "model-v1", "test-server-svc",
				).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "control plane service not found",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetControlPlaneServiceName(gomock.Any(), gomock.Any(), "test-server", "default").Return("", nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "control plane service not found for inference server test-server",
		},
		{
			name: "deployment route not configured",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetControlPlaneServiceName(gomock.Any(), gomock.Any(), "test-server", "default").Return("test-server-svc", nil)
				pp.EXPECT().CheckDeploymentRouteStatus(
					gomock.Any(), gomock.Any(), "test-deployment", "default", "test-server", "model-v1", "test-server-svc",
				).Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Deployment route is not configured",
		},
		{
			name: "check deployment route status fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetControlPlaneServiceName(gomock.Any(), gomock.Any(), "test-server", "default").Return("test-server-svc", nil)
				pp.EXPECT().CheckDeploymentRouteStatus(
					gomock.Any(), gomock.Any(), "test-deployment", "default", "test-server", "model-v1", "test-server-svc",
				).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to check deployment route status: api error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockProxy, mockGateway)

			actor := &TrafficRoutingActor{
				ProxyProvider: mockProxy,
				Gateway:       mockGateway,
				Logger:        zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

func TestTrafficRoutingRun(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		inputCondition          *api.Condition
		setupMocks              func(*proxymocks.MockProxyProvider, *gatewaysmocks.MockGateway)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "traffic routing configured successfully",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			inputCondition: createConditionWithServiceName("test-server-svc"),
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				pp.EXPECT().EnsureDeploymentRoute(
					gomock.Any(), gomock.Any(), "test-deployment", "default", "test-server", "model-v1", "test-server-svc",
				).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "missing inference server",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target:          nil,
				},
			},
			inputCondition:          &api.Condition{},
			setupMocks:              func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "inference server not specified for deployment test-deployment",
		},
		{
			name: "control plane service not found in metadata",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			inputCondition:          &api.Condition{}, // No metadata
			setupMocks:              func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "control plane service name not found in metadata for inference server test-server",
		},
		{
			name: "add deployment route fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			inputCondition: createConditionWithServiceName("test-server-svc"),
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				pp.EXPECT().EnsureDeploymentRoute(
					gomock.Any(), gomock.Any(), "test-deployment", "default", "test-server", "model-v1", "test-server-svc",
				).Return(errors.New("route creation failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to add deployment route: route creation failed",
		},
		{
			name: "traffic routing configured with complex deployment",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-deployment",
					Namespace: "production",
					Annotations: map[string]string{
						"rollout.michelangelo.ai/strategy": "rolling",
					},
				},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &api.ResourceIdentifier{Name: "bert_cola"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: "triton-server"},
					},
				},
			},
			inputCondition: createConditionWithServiceName("triton-server-svc"),
			setupMocks: func(pp *proxymocks.MockProxyProvider, gw *gatewaysmocks.MockGateway) {
				pp.EXPECT().EnsureDeploymentRoute(
					gomock.Any(), gomock.Any(), "complex-deployment", "production", "triton-server", "bert_cola", "triton-server-svc",
				).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockProxy, mockGateway)

			actor := &TrafficRoutingActor{
				ProxyProvider: mockProxy,
				Gateway:       mockGateway,
				Logger:        zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, tt.inputCondition)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
		})
	}
}

// createConditionWithServiceName creates a condition with the control plane service name in metadata.
func createConditionWithServiceName(serviceName string) *api.Condition {
	structVal := &types.Struct{
		Fields: map[string]*types.Value{
			"control_plane_service_name": {
				Kind: &types.Value_StringValue{StringValue: serviceName},
			},
		},
	}
	metadata, _ := types.MarshalAny(structVal)
	return &api.Condition{Metadata: metadata}
}
