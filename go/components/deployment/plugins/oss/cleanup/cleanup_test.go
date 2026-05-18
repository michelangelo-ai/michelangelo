package cleanup

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing/routingmocks"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// withSingleClusterAnnotation seeds the deployment's target-clusters snapshot annotation so the
// CleanupActor's per-cluster loops have something to iterate over.
func withSingleClusterAnnotation(t *testing.T, deployment *v2pb.Deployment, clusterID string) *v2pb.Deployment {
	t.Helper()
	target := &v2pb.ClusterTarget{
		ClusterId: clusterID,
		Connection: &v2pb.ClusterTarget_Kubernetes{
			Kubernetes: &v2pb.ConnectionSpec{
				Host: "https://kubernetes.default.svc",
				Port: "443",
			},
		},
	}
	if err := osscommon.WriteTargetClustersAnnotation(deployment, []*v2pb.ClusterTarget{target}); err != nil {
		t.Fatalf("seed target-clusters annotation: %v", err)
	}
	return deployment
}

func TestRetrieve(t *testing.T) {
	isName := "test-server"
	deploymentName := "test-deployment"
	namespace := "default"
	clusterID := "test-cluster"
	currentModel := "old-model"

	trafficRouteName := routenames.TrafficRouteName(isName)
	discoveryRouteName := routenames.DiscoveryRouteName(isName)
	trafficMatchPath := routenames.TrafficMatchPath(isName, deploymentName)
	trafficRewritePath := routenames.TrafficRewritePath(currentModel)
	discoveryMatchPath := routenames.DiscoveryMatchPath(isName, deploymentName)

	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider, *routingmocks.MockManager)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "model still exists in inference server, cleanup required",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{
					{Name: currentModel, StoragePath: "gs://bucket/old-model"},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Model old-model still exists in Inference Server",
		},
		{
			name: "unable to check model in inference server",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return(nil, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if model old-model exists in Inference Server: connection error",
		},
		{
			name: "TrafficRoute still exists, cleanup required",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{}, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRouteName, namespace,
					routing.Rule{MatchPath: trafficMatchPath, RewritePath: trafficRewritePath}).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Cleanup required: TrafficRoute for deployment test-deployment still exists in cluster test-cluster",
		},
		{
			name: "unable to check TrafficRoute exists",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{}, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRouteName, namespace,
					routing.Rule{MatchPath: trafficMatchPath, RewritePath: trafficRewritePath}).Return(false, errors.New("api error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if TrafficRoute exists for deployment test-deployment in cluster test-cluster: api error",
		},
		{
			name: "DiscoveryRoute still exists, cleanup required",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{}, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRouteName, namespace,
					routing.Rule{MatchPath: trafficMatchPath, RewritePath: trafficRewritePath}).Return(false, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRouteName, namespace,
					routing.Rule{MatchPath: discoveryMatchPath}).Return(true, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Cleanup required: DiscoveryRoute for deployment test-deployment still exists",
		},
		{
			name: "cleanup completed, all resources cleaned up",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{}, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), trafficRouteName, namespace,
					routing.Rule{MatchPath: trafficMatchPath, RewritePath: trafficRewritePath}).Return(false, nil)
				rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRouteName, namespace,
					routing.Rule{MatchPath: discoveryMatchPath}).Return(false, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			rm := routingmocks.NewMockManager(ctrl)
			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			tt.setupMocks(mockModelConfigProvider, rm)

			actor := &CleanupActor{
				ModelConfigProvider: mockModelConfigProvider,
				RouteManager:        rm,
				ClientFactory:       mockClientFactory,
				Logger:              zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Contains(t, condition.Reason, tt.expectedConditionReason)
		})
	}
}

func TestRun(t *testing.T) {
	isName := "test-server"
	deploymentName := "test-deployment"
	namespace := "default"
	clusterID := "test-cluster"
	currentModel := "old-model"

	trafficRouteName := routenames.TrafficRouteName(isName)
	discoveryRouteName := routenames.DiscoveryRouteName(isName)
	trafficMatchPath := routenames.TrafficMatchPath(isName, deploymentName)
	discoveryMatchPath := routenames.DiscoveryMatchPath(isName, deploymentName)

	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		setupMocks              func(*modelconfigmocks.MockModelConfigProvider, *routingmocks.MockManager)
		expectedConditionStatus api.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "successful cleanup, all operations complete",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, currentModel).Return(nil)
				rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), trafficRouteName, namespace, trafficMatchPath).Return(nil)
				rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRouteName, namespace, discoveryMatchPath).Return(nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
			expectedConditionReason: "",
		},
		{
			name: "model removal fails",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, currentModel).Return(errors.New("removal failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to unload old model old-model from inference server: removal failed",
		},
		{
			name: "TrafficRoute removal fails",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, currentModel).Return(nil)
				rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), trafficRouteName, namespace, trafficMatchPath).Return(errors.New("removal failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to remove TrafficRoute",
		},
		{
			name: "DiscoveryRoute removal fails",
			deployment: withSingleClusterAnnotation(t, &v2pb.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
				Spec: v2pb.DeploymentSpec{
					Target: &v2pb.DeploymentSpec_InferenceServer{
						InferenceServer: &api.ResourceIdentifier{Name: isName},
					},
				},
				Status: v2pb.DeploymentStatus{
					CurrentRevision: &api.ResourceIdentifier{Name: currentModel},
					Stage:           v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
				},
			}, clusterID),
			setupMocks: func(mcp *modelconfigmocks.MockModelConfigProvider, rm *routingmocks.MockManager) {
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, currentModel).Return(nil)
				rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), trafficRouteName, namespace, trafficMatchPath).Return(nil)
				rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRouteName, namespace, discoveryMatchPath).Return(errors.New("apply failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to remove DiscoveryRoute for deployment test-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			rm := routingmocks.NewMockManager(ctrl)
			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			tt.setupMocks(mockModelConfigProvider, rm)

			actor := &CleanupActor{
				ModelConfigProvider: mockModelConfigProvider,
				RouteManager:        rm,
				ClientFactory:       mockClientFactory,
				Logger:              zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			assert.Contains(t, condition.Reason, tt.expectedConditionReason)
		})
	}
}
