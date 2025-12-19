package cleanup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

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

// Retrieve checks if model ConfigMap entry and deployment HTTPRoute still exist.
func (a *CleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if cleanup is needed

	// check if model still exists in inference server
	if exists, err := a.gateway.CheckModelExists(
		ctx,
		a.logger,
		deployment.Status.GetCurrentRevision().GetName(),
		deployment.Spec.GetInferenceServer().GetName(),
		deployment.GetNamespace(),
		v2pb.BACKEND_TYPE_TRITON,
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

	exists, err := a.proxyProvider.DeploymentRouteExists(ctx, a.logger, deployment.Name, deployment.Namespace)
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

// Run removes model from ConfigMap and deletes the deployment HTTPRoute.
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

	// Initiate Unloading of Old Model From Inference Server
	if err := a.gateway.UnloadModel(ctx, a.logger, currentModel, inferenceServerName, resource.Namespace); err != nil {
		a.logger.Error("Failed to initiate unloading of old model", zap.Error(err), zap.String("operation", "unload_model"), zap.String("model", currentModel), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", resource.Namespace), zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelUnloadingFailed",
			Message: fmt.Sprintf("Failed to unload old model %s from inference server: %v", currentModel, err),
		}, nil
	}

	// PHASE 2: Delete DeploymentRoute
	// By removing the route, we will ensure that the model is no longer accessible.
	a.logger.Info("Phase 2: Deleting DeploymentRoute", zap.String("deploymentRoute", fmt.Sprintf("%s-httproute", resource.Name)))

	if err := a.proxyProvider.DeleteDeploymentRoute(ctx, a.logger, resource.Name, resource.Namespace); err != nil {
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
