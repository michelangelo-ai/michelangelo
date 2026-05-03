package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/discovery"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// CleanupActor removes models from ConfigMap and deletes deployment HTTPRoutes during deletion.
type CleanupActor struct {
	Client                 client.Client
	ClientFactory          clientfactory.ClientFactory
	RouteProvider          route.RouteProvider
	ModelDiscoveryProvider discovery.ModelDiscoveryProvider
	ModelConfigProvider    modelconfig.ModelConfigProvider
	Logger                 *zap.Logger
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

// Retrieve checks if model is still loaded in inference server and deployment HTTPRoute still exist.
func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// check if model still exists in inference server
	if exists, err := common.CheckModelExists(ctx, a.Logger, a.ModelConfigProvider, a.Client, deployment.Status.GetCurrentRevision().GetName(), deployment.Spec.GetInferenceServer().GetName(), deployment.GetNamespace()); err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckModelExists", fmt.Sprintf("Unable to check if model %s exists in Inference Server: %v", deployment.Status.CurrentRevision.Name, err)), nil
	} else if exists {
		return conditionUtils.GenerateFalseCondition(condition, "ModelStillExistsInInferenceServer", fmt.Sprintf("Model %s still exists in Inference Server", deployment.Status.CurrentRevision.Name)), nil
	}

	// Check the per-deployment HTTPRoute on every target cluster the rollout placed it in.
	// Cleanup is only complete when every cluster has had its route removed.
	targets, err := common.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToReadTargetClusters", fmt.Sprintf("Unable to read target-clusters annotation: %v", err)), nil
	}
	for _, target := range targets {
		clusterID := target.GetClusterId()
		dynClient, err := a.ClientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckDeploymentRouteExists", fmt.Sprintf("Unable to get dynamic client for cluster %s: %v", clusterID, err)), nil
		}
		exists, err := a.RouteProvider.DeploymentRouteExists(ctx, a.Logger, dynClient, deployment.Name, deployment.Namespace)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckDeploymentRouteExists", fmt.Sprintf("Unable to check if DeploymentRoute exists for deployment %s in cluster %s: %v", deployment.Name, clusterID, err)), nil
		}
		if exists {
			return conditionUtils.GenerateFalseCondition(condition, "DeploymentRouteStillExists", fmt.Sprintf("Cleanup required: DeploymentRoute %s still exists in cluster %s", deployment.Name, clusterID)), nil
		}
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run removes model from ConfigMap and deletes the deployment HTTPRoute.
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

	// Delete the per-cluster DeploymentRoutes that the rollout placed.
	targets, err := common.ReadTargetClustersAnnotation(resource)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToReadTargetClusters", fmt.Sprintf("Unable to read target-clusters annotation: %v", err)), nil
	}
	for _, target := range targets {
		clusterID := target.GetClusterId()
		dynClient, err := a.ClientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			return conditionUtils.GenerateFalseCondition(condition, "DeploymentRouteDeletionFailed", fmt.Sprintf("Failed to get dynamic client for cluster %s: %v", clusterID, err)), nil
		}
		a.Logger.Info("Deleting DeploymentRoute", zap.String("deploymentRoute", fmt.Sprintf("%s-httproute", resource.Name)), zap.String("cluster", clusterID))
		if err := a.RouteProvider.DeleteDeploymentRoute(ctx, a.Logger, dynClient, resource.Name, resource.Namespace); err != nil {
			if errors.IsNotFound(err) {
				a.Logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", fmt.Sprintf("%s-httproute", resource.Name)), zap.String("cluster", clusterID))
				continue
			}
			a.Logger.Error("Failed to delete HTTPRoute", zap.Error(err), zap.String("cluster", clusterID))
			return conditionUtils.GenerateFalseCondition(condition, "DeploymentRouteDeletionFailed", fmt.Sprintf("Failed to delete DeploymentRoute %s in cluster %s: %v", fmt.Sprintf("%s-httproute", resource.Name), clusterID, err)), nil
		}
	}

	// Delete the control-plane discovery route.
	if err := a.ModelDiscoveryProvider.DeleteDiscoveryRoute(ctx, resource.Name, resource.Namespace); err != nil {
		a.Logger.Error("Failed to delete discovery HTTPRoute", zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "DiscoveryRouteDeletionFailed", fmt.Sprintf("Failed to delete discovery route for deployment %s: %v", resource.Name, err)), nil
	}

	a.Logger.Info("Model cleanup completed successfully", zap.String("current_model", currentModel))
	return conditionUtils.GenerateTrueCondition(condition), nil
}
