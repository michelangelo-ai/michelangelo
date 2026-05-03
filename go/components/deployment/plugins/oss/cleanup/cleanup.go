package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/routes"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// CleanupActor removes models from ConfigMap and tears down the deployment's routing during deletion.
type CleanupActor struct {
	Client              client.Client
	DynamicClient       dynamic.Interface
	ClientFactory       clientfactory.ClientFactory
	RouteProvider       routes.RouteProvider
	ModelConfigProvider modelconfig.ModelConfigProvider
	Logger              *zap.Logger
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

// Retrieve checks if model is still loaded in inference server and the deployment's routing still exists.
func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// check if model still exists in inference server
	if exists, err := common.CheckModelExists(ctx, a.Logger, a.ModelConfigProvider, a.Client, deployment.Status.GetCurrentRevision().GetName(), deployment.Spec.GetInferenceServer().GetName(), deployment.GetNamespace()); err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckModelExists", fmt.Sprintf("Unable to check if model %s exists in Inference Server: %v", deployment.Status.CurrentRevision.Name, err)), nil
	} else if exists {
		return conditionUtils.GenerateFalseCondition(condition, "ModelStillExistsInInferenceServer", fmt.Sprintf("Model %s still exists in Inference Server", deployment.Status.CurrentRevision.Name)), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	currentModel := deployment.Status.GetCurrentRevision().GetName()

	// Check the per-cluster traffic route on every cluster the rollout placed the deployment in.
	// Cleanup is only complete when every cluster has had its rule removed.
	targets, err := common.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToReadTargetClusters", fmt.Sprintf("Unable to read target-clusters annotation: %v", err)), nil
	}
	for _, target := range targets {
		clusterID := target.GetClusterId()
		dynClient, err := a.ClientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckTrafficRouteExists", fmt.Sprintf("Unable to get dynamic client for cluster %s: %v", clusterID, err)), nil
		}
		exists, err := a.RouteProvider.DeploymentTrafficRouteExists(ctx, dynClient, clusterID, inferenceServerName, deployment.Namespace, deployment.Name, currentModel)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckTrafficRouteExists", fmt.Sprintf("Unable to check if TrafficRoute exists for deployment %s in cluster %s: %v", deployment.Name, clusterID, err)), nil
		}
		if exists {
			return conditionUtils.GenerateFalseCondition(condition, "TrafficRouteStillExists", fmt.Sprintf("Cleanup required: TrafficRoute for deployment %s still exists in cluster %s", deployment.Name, clusterID)), nil
		}
	}

	// Check the control-plane discovery route.
	if exists, err := a.RouteProvider.DeploymentDiscoveryRouteExists(ctx, a.DynamicClient, inferenceServerName, deployment.Namespace, deployment.Name); err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckDiscoveryRouteExists", fmt.Sprintf("Unable to check if DiscoveryRoute exists for deployment %s: %v", deployment.Name, err)), nil
	} else if exists {
		return conditionUtils.GenerateFalseCondition(condition, "DiscoveryRouteStillExists", fmt.Sprintf("Cleanup required: DiscoveryRoute for deployment %s still exists", deployment.Name)), nil
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run removes the model from ConfigMap and tears down the deployment's routing.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running cleanup for deployment", zap.String("deployment", resource.Name))

	a.Logger.Info("Cleaning up model artifacts and ConfigMaps", zap.String("deployment", resource.Name))

	currentModel := resource.Status.CurrentRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name

	a.Logger.Info("Starting model cleanup",
		zap.String("current_model", currentModel),
		zap.String("inference_server", inferenceServerName))

	// Initiate unloading of old model from inference server
	a.Logger.Info("Unloading old model from inference server", zap.String("old_model", currentModel))
	if err := a.ModelConfigProvider.RemoveModelFromConfig(ctx, a.Logger, a.Client, inferenceServerName, resource.Namespace, currentModel); err != nil {
		a.Logger.Error("Failed to initiate unloading of old model", zap.Error(err), zap.String("operation", "unload_model"), zap.String("model", currentModel), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", resource.Namespace), zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
		return conditionUtils.GenerateFalseCondition(condition, "ModelUnloadingFailed", fmt.Sprintf("Failed to unload old model %s from inference server: %v", currentModel, err)), nil
	}

	// Remove the per-cluster TrafficRoute rules that the rollout placed.
	targets, err := common.ReadTargetClustersAnnotation(resource)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToReadTargetClusters", fmt.Sprintf("Unable to read target-clusters annotation: %v", err)), nil
	}
	for _, target := range targets {
		clusterID := target.GetClusterId()
		dynClient, err := a.ClientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "TrafficRouteRemovalFailed", fmt.Sprintf("Failed to get dynamic client for cluster %s: %v", clusterID, err)), nil
		}
		a.Logger.Info("Removing TrafficRoute for deployment", zap.String("deployment", resource.Name), zap.String("cluster", clusterID))
		if err := a.RouteProvider.RemoveTrafficRule(ctx, dynClient, clusterID, inferenceServerName, resource.Namespace, resource.Name); err != nil {
			a.Logger.Error("Failed to remove TrafficRoute", zap.Error(err), zap.String("cluster", clusterID))
			return conditionUtils.GenerateFalseCondition(condition, "TrafficRouteRemovalFailed", fmt.Sprintf("Failed to remove TrafficRoute for deployment %s in cluster %s: %v", resource.Name, clusterID, err)), nil
		}
	}

	// Remove the control-plane DiscoveryRoute rule.
	if err := a.RouteProvider.RemoveDiscoveryRule(ctx, a.DynamicClient, inferenceServerName, resource.Namespace, resource.Name); err != nil {
		a.Logger.Error("Failed to remove DiscoveryRoute", zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "DiscoveryRouteRemovalFailed", fmt.Sprintf("Failed to remove DiscoveryRoute for deployment %s: %v", resource.Name, err)), nil
	}

	a.Logger.Info("Model cleanup completed successfully", zap.String("current_model", currentModel))
	return conditionUtils.GenerateTrueCondition(condition), nil
}
