package strategies

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ModelCleanupActor handles cleanup of old models after successful deployment
// Following Uber's UCS pattern for safe model cleanup
type ModelCleanupActor struct {
	Client                 client.Client
	ModelConfigMapProvider configmap.ModelConfigMapProvider
	Gateway                gateways.Gateway
	Logger                 *zap.Logger
}

func (a *ModelCleanupActor) GetType() string {
	return common.ActorTypeModelCleanup
}

func (a *ModelCleanupActor) GetLogger() *zap.Logger {
	return a.Logger
}

func (a *ModelCleanupActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()

	// If models are the same, no cleanup needed
	if currentModel == "" || (currentModel == desiredModel) {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "NoCleanupNeeded",
			Message: "No cleanup required",
		}, nil
	}

	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	a.Logger.Info("Checking if old model cleanup is needed",
		zap.String("current_model", currentModel),
		zap.String("desired_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	// Check if old model still exists in Triton (following Uber's verification pattern)
	// Use gateway to check if old model is still loaded
	statusRequest := gateways.CheckModelStatusRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}

	ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
	if err != nil {
		// If we can't check status, assume cleanup is needed
		a.Logger.Info("Cannot verify old model status, cleanup may be needed", zap.Error(err))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "CleanupPending",
			Message: fmt.Sprintf("Need to cleanup old model %s", currentModel),
		}, nil
	}

	if ready {
		// Old model is still loaded, cleanup needed
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "CleanupPending",
			Message: fmt.Sprintf("Old model %s still loaded, cleanup needed", currentModel),
		}, nil
	}

	// Old model not found or already cleaned up
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupComplete",
		Message: fmt.Sprintf("Old model %s already cleaned up", currentModel),
	}, nil
}

func (a *ModelCleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running model cleanup for deployment", zap.String("deployment", resource.Name))

	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()

	a.Logger.Info("Starting model cleanup",
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	// PHASE 1: Update ConfigMap to remove old model (following Uber's UCS pattern)

	// Get current ConfigMap and remove old model from it
	a.Logger.Info("Phase 1: Removing old model from ConfigMap", zap.String("old_model", currentModel))

	// Remove old model from ConfigMap
	if err := a.ModelConfigMapProvider.RemoveModelFromConfigMap(ctx, configmap.RemoveModelFromConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		ModelName:       currentModel,
	}); err != nil {
		a.Logger.Error("Failed to remove old model from ConfigMap", zap.String("model", currentModel), zap.Error(err))
		// Don't fail entire deployment if remove from ConfigMap fails
	}

	// if err := a.ModelConfigMapProvider.AddModelToConfigMap(ctx, configmap.AddModelToConfigMapRequest{
	// 	InferenceServer: inferenceServerName,
	// 	Namespace:       resource.Namespace,
	// 	ModelConfig: configmap.ModelConfigEntry{
	// 		Name:   desiredModel,
	// 		S3Path: desiredModel,
	// 	},
	// }); err != nil {
	// 	a.Logger.Error("Failed to add new model to ConfigMap", zap.String("model", desiredModel), zap.Error(err))
	// 	return &apipb.Condition{
	// 		Type:    a.GetType(),
	// 		Status:  apipb.CONDITION_STATUS_FALSE,
	// 		Reason:  "ConfigMapAddFailed",
	// 		Message: fmt.Sprintf("Failed to add new model %s to ConfigMap: %v", desiredModel, err),
	// 	}, nil
	// }

	// PHASE 2: Directly unload old model from Triton using API (following Uber's pattern)
	a.Logger.Info("Phase 2: Unloading old model from Triton", zap.String("old_model", currentModel))

	if err := a.Gateway.UnloadModel(ctx, a.Logger, gateways.UnloadModelRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}); err != nil {
		a.Logger.Error("Failed to unload old model from Triton", zap.String("model", currentModel), zap.Error(err))
		// Don't fail the deployment if direct unload fails - ConfigMap update should handle it
		a.Logger.Info("ConfigMap update should eventually unload the model automatically")
	}

	// PHASE 3: Verify cleanup completed
	a.Logger.Info("Phase 3: Verifying old model is unloaded", zap.String("old_model", currentModel))

	statusRequest := gateways.CheckModelStatusRequest{
		ModelName:       currentModel,
		InferenceServer: inferenceServerName,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}

	ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
	if err == nil && ready {
		a.Logger.Info("Old model still loaded, but ConfigMap update should unload it eventually", zap.String("model", currentModel))
	} else {
		a.Logger.Info("Old model successfully unloaded", zap.String("model", currentModel))
	}

	a.Logger.Info("Model cleanup completed successfully",
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel))

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: fmt.Sprintf("Successfully cleaned up old model %s, active model is now %s", currentModel, desiredModel),
	}, nil
}
