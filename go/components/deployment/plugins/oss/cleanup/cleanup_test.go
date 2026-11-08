package cleanup

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

func newKubeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
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
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return([]modelconfig.ModelConfigEntry{
					{Name: currentModel, StoragePath: "gs://bucket/old-model", DeploymentName: deploymentName},
				}, nil)
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Model old-model still exists in Inference Server in cluster test-cluster",
		},
		{
			name: "unable to check model in inference server",
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
				mcp.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).Return(nil, errors.New("connection error"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Unable to check if model old-model exists in Inference Server in cluster test-cluster: connection error",
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
			mockClientFactory.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			tt.setupMocks(mockModelConfigProvider, rm)

			actor := &CleanupActor{
				Client:              newKubeClient(t),
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
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).Return(nil)
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
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).Return(errors.New("removal failed"))
			},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedConditionReason: "Failed to unload old model old-model from inference server in cluster test-cluster: removal failed",
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
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).Return(nil)
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
				mcp.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).Return(nil)
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
			mockClientFactory.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			tt.setupMocks(mockModelConfigProvider, rm)

			actor := &CleanupActor{
				Client:              newKubeClient(t),
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

// TestRunUnloadsModelFromEveryCluster guards the multi-cluster deletion path: the model
// config is a per-cluster ConfigMap, so removing it only from the control plane leaves
// remote inference servers serving the model after the Deployment is gone.
func TestRunUnloadsModelFromEveryCluster(t *testing.T) {
	isName := "test-server"
	deploymentName := "test-deployment"
	namespace := "default"
	currentModel := "old-model"
	clusterIDs := []string{"cluster-a", "cluster-b"}

	targets := make([]*v2pb.ClusterTarget, 0, len(clusterIDs))
	for _, id := range clusterIDs {
		targets = append(targets, &v2pb.ClusterTarget{
			ClusterId: id,
			Connection: &v2pb.ClusterTarget_Kubernetes{
				Kubernetes: &v2pb.ConnectionSpec{Host: "https://kubernetes.default.svc", Port: "443"},
			},
		})
	}

	deployment := &v2pb.Deployment{
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
	}
	if err := osscommon.WriteTargetClustersAnnotation(deployment, targets); err != nil {
		t.Fatalf("seed target-clusters annotation: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
	rm := routingmocks.NewMockManager(ctrl)
	mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)

	// One client per target cluster, not one control-plane client reused.
	for _, id := range clusterIDs {
		matchesCluster := clusterTargetMatcher(id)
		mockClientFactory.EXPECT().GetClient(gomock.Any(), matchesCluster).Return(nil, nil)
		mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), matchesCluster).Return(nil, nil)
	}

	mockModelConfigProvider.EXPECT().
		RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).
		Return(nil).Times(len(clusterIDs))
	rm.EXPECT().
		RemoveRules(gomock.Any(), gomock.Any(), routenames.TrafficRouteName(isName), namespace,
			routenames.TrafficMatchPath(isName, deploymentName)).
		Return(nil).Times(len(clusterIDs))
	rm.EXPECT().
		RemoveRules(gomock.Any(), gomock.Any(), routenames.DiscoveryRouteName(isName), namespace,
			routenames.DiscoveryMatchPath(isName, deploymentName)).
		Return(nil)

	actor := &CleanupActor{
		Client:              newKubeClient(t),
		ModelConfigProvider: mockModelConfigProvider,
		RouteManager:        rm,
		ClientFactory:       mockClientFactory,
		Logger:              zap.NewNop(),
	}

	condition, err := actor.Run(context.Background(), deployment, &api.Condition{})

	assert.NoError(t, err)
	assert.Equal(t, api.CONDITION_STATUS_TRUE, condition.Status)
}

func TestCleanup_TargetClusterConnection(t *testing.T) {
	isName := "test-server"
	namespace := "default"
	deploymentName := "test-deployment"
	currentModel := "old-model"

	remoteTarget := func(version string) *v2pb.ClusterTarget {
		return &v2pb.ClusterTarget{
			ClusterId: "remote",
			Connection: &v2pb.ClusterTarget_Kubernetes{Kubernetes: &v2pb.ConnectionSpec{
				Host:      "https://remote.example.com",
				Port:      "443",
				TokenTag:  "remote-token-" + version,
				CaDataTag: "remote-ca-" + version,
			}},
		}
	}
	newDeployment := func(t *testing.T) *v2pb.Deployment {
		deployment := &v2pb.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: deploymentName, Namespace: namespace},
			Spec: v2pb.DeploymentSpec{
				Target: &v2pb.DeploymentSpec_InferenceServer{
					InferenceServer: &api.ResourceIdentifier{Name: isName},
				},
			},
			Status: v2pb.DeploymentStatus{CurrentRevision: &api.ResourceIdentifier{Name: currentModel}},
		}
		require.NoError(t, osscommon.WriteTargetClustersAnnotation(deployment,
			[]*v2pb.ClusterTarget{remoteTarget("v1")}))
		return deployment
	}
	renamedInferenceServer := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: isName, Namespace: namespace},
		Spec:       v2pb.InferenceServerSpec{ClusterTargets: []*v2pb.ClusterTarget{remoteTarget("v2")}},
	}

	tests := []struct {
		name       string
		kubeClient func(t *testing.T) client.Client
		wantToken  string
	}{
		{
			name:       "secrets renamed since rollout uses the inference server connection",
			kubeClient: func(t *testing.T) client.Client { return newKubeClient(t, renamedInferenceServer) },
			wantToken:  "remote-token-v2",
		},
		{
			name: "inference server read failure uses the recorded snapshot",
			kubeClient: func(t *testing.T) client.Client {
				scheme := runtime.NewScheme()
				require.NoError(t, v2pb.AddToScheme(scheme))
				return fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return errors.New("cache not synced")
					},
				}).Build()
			},
			wantToken: "remote-token-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/retrieve", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			rm := routingmocks.NewMockManager(ctrl)
			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher(tt.wantToken)).Return(nil, nil)
			mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), tokenTagMatcher(tt.wantToken)).Return(nil, nil)
			mockModelConfigProvider.EXPECT().
				GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).
				Return(nil, nil)
			rm.EXPECT().RuleExists(gomock.Any(), gomock.Any(), gomock.Any(), namespace, gomock.Any()).
				Return(false, nil).Times(2)
			actor := NewCleanupPlugin(Params{
				Client:              tt.kubeClient(t),
				ClientFactory:       mockClientFactory,
				RouteManager:        rm,
				ModelConfigProvider: mockModelConfigProvider,
				Logger:              zap.NewNop(),
			}).GetActors()[0]

			condition, err := actor.Retrieve(context.Background(), newDeployment(t), &api.Condition{})

			require.NoError(t, err)
			assert.Equal(t, api.CONDITION_STATUS_TRUE, condition.Status)
		})

		t.Run(tt.name+"/run", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockModelConfigProvider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			rm := routingmocks.NewMockManager(ctrl)
			mockClientFactory := clientfactorymocks.NewMockClientFactory(ctrl)
			mockClientFactory.EXPECT().GetClient(gomock.Any(), tokenTagMatcher(tt.wantToken)).Return(nil, nil)
			mockClientFactory.EXPECT().GetDynamicClient(gomock.Any(), tokenTagMatcher(tt.wantToken)).Return(nil, nil)
			mockModelConfigProvider.EXPECT().
				RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace, deploymentName, currentModel).
				Return(nil)
			rm.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), gomock.Any(), namespace, gomock.Any()).
				Return(nil).Times(2)
			actor := NewCleanupPlugin(Params{
				Client:              tt.kubeClient(t),
				ClientFactory:       mockClientFactory,
				RouteManager:        rm,
				ModelConfigProvider: mockModelConfigProvider,
				Logger:              zap.NewNop(),
			}).GetActors()[0]

			condition, err := actor.Run(context.Background(), newDeployment(t), &api.Condition{})

			require.NoError(t, err)
			assert.Equal(t, api.CONDITION_STATUS_TRUE, condition.Status)
		})
	}
}

func TestCleanup_UnreadableSnapshot_ReportsWithoutClusterLookups(t *testing.T) {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-deployment",
			Namespace:   "default",
			Annotations: map[string]string{osscommon.TargetClustersAnnotation: "{not json}"},
		},
		Spec: v2pb.DeploymentSpec{
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &api.ResourceIdentifier{Name: "test-server"},
			},
		},
		Status: v2pb.DeploymentStatus{CurrentRevision: &api.ResourceIdentifier{Name: "old-model"}},
	}
	steps := map[string]func(*CleanupActor) (*api.Condition, error){
		"retrieve": func(a *CleanupActor) (*api.Condition, error) {
			return a.Retrieve(context.Background(), deployment, &api.Condition{})
		},
		"run": func(a *CleanupActor) (*api.Condition, error) {
			return a.Run(context.Background(), deployment, &api.Condition{})
		},
	}

	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			core, logs := observer.New(zap.WarnLevel)
			actor := &CleanupActor{
				Client:        newKubeClient(t),
				ClientFactory: clientfactorymocks.NewMockClientFactory(ctrl),
				Logger:        zap.New(core),
			}

			condition, err := step(actor)

			require.NoError(t, err)
			assert.Equal(t, api.CONDITION_STATUS_FALSE, condition.Status)
			assert.Equal(t, "UnableToReadTargetClusters", condition.Message)
			assert.Zero(t, logs.Len(), "an unreadable snapshot is not an InferenceServer read failure")
		})
	}
}

// tokenTagMatcher matches a ClusterTarget argument by the name of its token Secret.
type tokenTagMatcher string

func (m tokenTagMatcher) Matches(x interface{}) bool {
	target, ok := x.(*v2pb.ClusterTarget)
	return ok && target.GetKubernetes().GetTokenTag() == string(m)
}

func (m tokenTagMatcher) String() string {
	return "cluster target with token Secret " + string(m)
}

// clusterTargetMatcher matches a ClusterTarget argument by cluster ID.
type clusterTargetMatcher string

func (m clusterTargetMatcher) Matches(x interface{}) bool {
	target, ok := x.(*v2pb.ClusterTarget)
	return ok && target.GetClusterId() == string(m)
}

func (m clusterTargetMatcher) String() string {
	return "cluster target " + string(m)
}
