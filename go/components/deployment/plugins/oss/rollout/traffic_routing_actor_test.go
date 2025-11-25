package rollout

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy/proxymocks"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestTrafficRoutingRetrieve(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*proxymocks.MockProxyProvider)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().CheckDeploymentRouteStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "TrafficRoutingConfigured",
			expectedConditionMessage: "HTTPRoute test-deployment successfully configured for deployment",
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().CheckDeploymentRouteStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "DeploymentRouteNotConfigured",
			expectedConditionMessage: "Deployment route is not configured",
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().CheckDeploymentRouteStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "CheckDeploymentRouteStatusFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			tt.setupMocks(mockProxy)

			actor := &TrafficRoutingActor{
				ProxyProvider: mockProxy,
				Logger:        zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			if tt.expectedConditionMessage != "" {
				assert.Contains(t, condition.Message, tt.expectedConditionMessage)
			}
		})
	}
}

func TestTrafficRoutingRun(t *testing.T) {
	tests := []struct {
		name                     string
		deployment               *v2pb.Deployment
		setupMocks               func(*proxymocks.MockProxyProvider)
		expectedConditionStatus  api.ConditionStatus
		expectedConditionReason  string
		expectedConditionMessage string
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().EnsureDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "TrafficRoutingConfigured",
			expectedConditionMessage: "HTTPRoute for deployment test-deployment successfully configured",
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				// No mock setup needed - early return
			},
			expectedConditionStatus:  api.CONDITION_STATUS_FALSE,
			expectedConditionReason:  "MissingInferenceServer",
			expectedConditionMessage: "inference server not specified for deployment test-deployment",
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().EnsureDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("route creation failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "AddDeploymentRouteFailed",
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
			setupMocks: func(pp *proxymocks.MockProxyProvider) {
				pp.EXPECT().EnsureDeploymentRoute(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedConditionStatus:  api.CONDITION_STATUS_TRUE,
			expectedConditionReason:  "TrafficRoutingConfigured",
			expectedConditionMessage: "HTTPRoute for deployment complex-deployment successfully configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProxy := proxymocks.NewMockProxyProvider(ctrl)
			tt.setupMocks(mockProxy)

			actor := &TrafficRoutingActor{
				ProxyProvider: mockProxy,
				Logger:        zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, nil)

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)
			if tt.expectedConditionMessage != "" {
				assert.Contains(t, condition.Message, tt.expectedConditionMessage)
			}
		})
	}
}
