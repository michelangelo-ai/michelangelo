package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// CleanupActor handles cleanup operations following Uber patterns
type CleanupActor struct {
	proxyProvider          proxy.ProxyProvider
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if cleanup is needed
	// check if model still exists in ConfigMap
	if exists, err := modelExistsInConfig(
		ctx,
		a.modelConfigMapProvider,
		deployment.Spec.GetInferenceServer().Name,
		deployment.Namespace,
		deployment.Status.CurrentRevision.Name,
	); err != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "UnableToCheckModelInConfigMap",
			Message: fmt.Sprintf("Unable to check if model %s exists in ConfigMap: %v", deployment.Status.CurrentRevision.Name, err),
		}, nil
	} else if exists {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelStillExistsInConfigMap",
			Message: fmt.Sprintf("Model %s still exists in ConfigMap", deployment.Status.CurrentRevision.Name),
		}, nil
	}

	exists, err := a.proxyProvider.DeploymentRouteExists(ctx, a.logger, proxy.DeploymentRouteExistsRequest{
		DeploymentName: deployment.Name,
		Namespace:      deployment.Namespace,
	})
	if err != nil {
		// assume cleanup is required if we cannot check if the route exists
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "UnableToCheckHTTPRouteExists",
			Message: fmt.Sprintf("Unable to check if HTTPRoute %s exists: %v", fmt.Sprintf("%s-httproute", deployment.Name), err),
		}, nil
	} else if exists {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HTTPRouteStillExists",
			Message: fmt.Sprintf("Cleanup required: HTTPRoute %s still exists", fmt.Sprintf("%s-httproute", deployment.Name)),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Cleanup not required",
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running cleanup for deployment", zap.String("deployment", resource.Name))

	// Update deployment status to indicate cleanup is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS

	a.logger.Info("Cleaning up model artifacts and ConfigMaps", zap.String("deployment", resource.Name))

	currentModel := resource.Status.CurrentRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name

	a.logger.Info("Starting model cleanup",
		zap.String("current_model", currentModel),
		zap.String("inference_server", inferenceServerName))

	// PHASE 1: Update ConfigMap to remove old models
	// Get current ConfigMap and remove old model from it
	a.logger.Info("Phase 1: Removing old model from ConfigMap", zap.String("old_model", currentModel))

	// Remove old model from ConfigMap
	if err := a.modelConfigMapProvider.RemoveModelFromConfigMap(ctx, configmap.RemoveModelFromConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		ModelName:       currentModel,
	}); err != nil {
		a.logger.Error("Failed to remove old model from ConfigMap", zap.String("model", currentModel), zap.Error(err))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ConfigMapCleanupFailed",
			Message: fmt.Sprintf("Failed to remove old model %s from ConfigMap: %v", currentModel, err),
		}, nil
	}

	// PHASE 2: Directly unload old model from Triton using API
	a.logger.Info("Phase 2: Unloading old model from Triton", zap.String("old_model", currentModel))

	if err := a.gateway.UnloadModel(ctx, a.logger, gateways.UnloadModelRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
	}); err != nil {
		a.logger.Error("Failed to unload old model from Triton", zap.String("model", currentModel), zap.Error(err))
		// ConfigMap update should eventually unload the model automatically, hence we will not fail the deployment
		a.logger.Info("ConfigMap update should eventually unload the model automatically")
	}

	// PHASE 3: Verify model is unloaded
	a.logger.Info("Phase 3: Verifying old model is unloaded", zap.String("old_model", currentModel))

	statusRequest := gateways.CheckModelStatusRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
	}

	ready, err := a.gateway.CheckModelStatus(ctx, a.logger, statusRequest)
	if err == nil && ready {
		a.logger.Info("Old model still loaded, but ConfigMap update should unload it eventually", zap.String("model", currentModel))
	} else {
		a.logger.Info("Old model successfully unloaded", zap.String("model", currentModel))
	}

	if err := a.proxyProvider.DeleteDeploymentRoute(ctx, a.logger, proxy.DeleteDeploymentRouteRequest{
		DeploymentName: resource.Name,
		Namespace:      resource.Namespace,
	}); err != nil {
		a.logger.Error("Failed to delete HTTPRoute", zap.Error(err))
		if errors.IsNotFound(err) {
			a.logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", fmt.Sprintf("%s-httproute", resource.Name)))
		} else {
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "HTTPRouteCleanupFailed",
				Message: fmt.Sprintf("Failed to delete HTTPRoute %s: %v", fmt.Sprintf("%s-httproute", resource.Name), err),
			}, nil
		}
	}

	a.logger.Info("Model cleanup completed successfully", zap.String("current_model", currentModel))

	// Mark cleanup as complete
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
	a.logger.Info("Cleanup completed for OSS deployment")

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Cleanup completed successfully",
	}, nil
}

func modelExistsInConfig(ctx context.Context, provider configmap.ModelConfigMapProvider, inferenceServerName, namespace, modelName string) (bool, error) {
	currentConfigs, err := provider.GetModelsFromConfigMap(ctx, configmap.GetModelConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get current model config: %w", err)
	}

	for _, config := range currentConfigs {
		if config.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}
