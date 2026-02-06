package steadystate

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
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/gatewaysmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestSteadyStateActor_Retrieve(t *testing.T) {
	testCluster := &gateways.TargetClusterConnection{
		ClusterId: "test-cluster",
		Host:      "host1",
	}

	tests := []struct {
		name            string
		deployment      *v2pb.Deployment
		setupMocks      func(*gatewaysmocks.MockGateway)
		expectedStatus  apipb.ConditionStatus
		expectedMessage string
	}{
		{
			name: "steady state when inference server and model are healthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
				gw.EXPECT().InferenceServerIsHealthy(
					gomock.Any(), gomock.Any(), "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedMessage: "",
		},
		{
			name: "GetDeploymentTargetInfo fails",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(nil, errors.New("not found"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "GetDeploymentTargetInfoFailed",
		},
		{
			name: "inference server is unhealthy",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
				gw.EXPECT().InferenceServerIsHealthy(
					gomock.Any(), gomock.Any(), "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HealthCheckFailed",
		},
		{
			name: "inference server health check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
				gw.EXPECT().InferenceServerIsHealthy(
					gomock.Any(), gomock.Any(), "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(false, errors.New("connection error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "HealthCheckFailed",
		},
		{
			name: "model is not ready",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
				gw.EXPECT().InferenceServerIsHealthy(
					gomock.Any(), gomock.Any(), "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(false, nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "ModelHealthCheckFailed",
		},
		{
			name: "model status check fails with error",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			setupMocks: func(gw *gatewaysmocks.MockGateway) {
				gw.EXPECT().GetDeploymentTargetInfo(gomock.Any(), gomock.Any(), "test-server", "default").
					Return(&gateways.DeploymentTargetInfo{
						BackendType:    v2pb.BACKEND_TYPE_TRITON,
						ClusterTargets: []*gateways.TargetClusterConnection{testCluster},
					}, nil)
				gw.EXPECT().InferenceServerIsHealthy(
					gomock.Any(), gomock.Any(), "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(true, nil)
				gw.EXPECT().CheckModelStatus(
					gomock.Any(), gomock.Any(), "model-v1", "test-server", "default", testCluster, v2pb.BACKEND_TYPE_TRITON,
				).Return(false, errors.New("api error"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "ModelHealthCheckFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGateway := gatewaysmocks.NewMockGateway(ctrl)
			tt.setupMocks(mockGateway)

			actor := &SteadyStateActor{
				gateway: mockGateway,
				logger:  zap.NewNop(),
			}

			result, err := actor.Retrieve(context.Background(), tt.deployment, &apipb.Condition{})

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, result.Message)
			}
		})
	}
}

func TestSteadyStateActor_Run(t *testing.T) {
	tests := []struct {
		name           string
		deployment     *v2pb.Deployment
		inputCondition *apipb.Condition
		expectedStatus apipb.ConditionStatus
		expectedReason string
	}{
		{
			name: "run returns the input condition unchanged",
			deployment: &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
				Spec: v2pb.DeploymentSpec{
					DesiredRevision: &apipb.ResourceIdentifier{Name: "model-v1"},
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &apipb.ResourceIdentifier{Name: "test-server"},
					},
				},
			},
			inputCondition: &apipb.Condition{
				Status: apipb.CONDITION_STATUS_FALSE,
				Reason: "HealthCheckFailed",
			},
			expectedStatus: apipb.CONDITION_STATUS_FALSE,
			expectedReason: "HealthCheckFailed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &SteadyStateActor{
				logger: zap.NewNop(),
			}

			result, err := actor.Run(context.Background(), tt.deployment, tt.inputCondition)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedReason, result.Reason)
		})
	}
}
