package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// CleanupActor removes models from ConfigMap and deletes deployment HTTPRoutes during deletion.
type CleanupActor struct {
	proxyProvider proxy.ProxyProvider
	gateway       gateways.Gateway
	logger        *zap.Logger
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

// Retrieve checks if model is still loaded in inference server and deployment HTTPRoute still exist.
func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// check if model still exists in inference server
	if exists, err := a.gateway.CheckModelExists(ctx, a.logger, deployment.Status.GetCurrentRevision().GetName(), deployment.Spec.GetInferenceServer().GetName(), deployment.GetNamespace(), v2pb.BACKEND_TYPE_TRITON); err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckModelExists", fmt.Sprintf("Unable to check if model %s exists in Inference Server: %v", deployment.Status.CurrentRevision.Name, err)), nil
	} else if exists {
		return conditionUtils.GenerateFalseCondition(condition, "ModelStillExistsInInferenceServer", fmt.Sprintf("Model %s still exists in Inference Server", deployment.Status.CurrentRevision.Name)), nil
	}

	exists, err := a.proxyProvider.DeploymentRouteExists(ctx, a.logger, deployment.Name, deployment.Namespace)
	if err != nil {
		// assume cleanup is required if we cannot check if the route exists
		return conditionUtils.GenerateFalseCondition(condition, "UnableToCheckHTTPRouteExists", fmt.Sprintf("Unable to check if HTTPRoute %s exists: %v", fmt.Sprintf("%s-httproute", deployment.Name), err)), nil
	} else if exists {
		return conditionUtils.GenerateFalseCondition(condition, "HTTPRouteStillExists", fmt.Sprintf("Cleanup required: HTTPRoute %s still exists", fmt.Sprintf("%s-httproute", deployment.Name))), nil
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run removes model from ConfigMap and deletes the deployment HTTPRoute.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running cleanup for deployment", zap.String("deployment", resource.Name))

	a.logger.Info("Cleaning up model artifacts and ConfigMaps", zap.String("deployment", resource.Name))

	currentModel := resource.Status.CurrentRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name

	a.logger.Info("Starting model cleanup",
		zap.String("current_model", currentModel),
		zap.String("inference_server", inferenceServerName))

	// Initiate unloading of old model from inference server
	a.logger.Info("Unloading old model from inference server", zap.String("old_model", currentModel))
	if err := a.gateway.UnloadModel(ctx, a.logger, currentModel, inferenceServerName, resource.Namespace, v2pb.BACKEND_TYPE_TRITON); err != nil {
		a.logger.Error("Failed to initiate unloading of old model", zap.Error(err), zap.String("operation", "unload_model"), zap.String("model", currentModel), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", resource.Namespace), zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
		return conditionUtils.GenerateFalseCondition(condition, "ModelUnloadingFailed", fmt.Sprintf("Failed to unload old model %s from inference server: %v", currentModel, err)), nil
	}

	// Delete DeploymentRoute to ensure the model is no longer accessible
	a.logger.Info("Deleting DeploymentRoute", zap.String("deploymentRoute", fmt.Sprintf("%s-httproute", resource.Name)))
	if err := a.proxyProvider.DeleteDeploymentRoute(ctx, a.logger, resource.Name, resource.Namespace); err != nil {
		a.logger.Error("Failed to delete HTTPRoute", zap.Error(err))
		if errors.IsNotFound(err) {
			a.logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", fmt.Sprintf("%s-httproute", resource.Name)))
		} else {
			return conditionUtils.GenerateFalseCondition(condition, "HTTPRouteCleanupFailed", fmt.Sprintf("Failed to delete HTTPRoute %s: %v", fmt.Sprintf("%s-httproute", resource.Name), err)), nil
		}
	}

	a.logger.Info("Model cleanup completed successfully", zap.String("current_model", currentModel))
	return conditionUtils.GenerateTrueCondition(condition), nil
}
