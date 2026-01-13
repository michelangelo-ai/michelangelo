package strategies

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelCleanupActor{}

// ModelCleanupActor unloads previous model versions from inference servers after successful rollout.
type ModelCleanupActor struct {
	Gateway gateways.Gateway
	Logger  *zap.Logger
}

// GetType returns the condition type identifier for model cleanup.
func (a *ModelCleanupActor) GetType() string {
	return common.ActorTypeModelCleanup
}

// GetLogger returns the logger instance for this actor.
func (a *ModelCleanupActor) GetLogger() *zap.Logger {
	return a.Logger
}

// Retrieve checks if old models are still loaded and require cleanup.
func (a *ModelCleanupActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()
	// If models are the same, no cleanup needed
	if currentModel == "" || (currentModel == desiredModel) {
		return conditionUtils.GenerateTrueCondition(condition), nil
	}

	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	a.Logger.Info("Checking if old model cleanup is needed",
		zap.String("current_model", currentModel),
		zap.String("desired_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	// Check if old model still exists in Triton
	ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, currentModel, inferenceServerName, resource.Namespace, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		a.Logger.Info("Cannot verify old model status, cleanup may be needed", zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "CleanupPending", fmt.Sprintf("Need to cleanup old model %s", currentModel)), nil
	}

	if ready {
		// Old model is still loaded, cleanup needed
		return conditionUtils.GenerateFalseCondition(condition, "CleanupPending", fmt.Sprintf("Old model %s still loaded, cleanup needed", currentModel)), nil
	}
	// Old model not found or already cleaned up
	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run removes old model from ConfigMap and directly unloads it from Triton via API.
func (a *ModelCleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running model cleanup for deployment", zap.String("deployment", resource.Name))

	currentModel := resource.Status.GetCurrentRevision().GetName()
	desiredModel := resource.Spec.GetDesiredRevision().GetName()
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()

	a.Logger.Info("Starting old model cleanup",
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel),
		zap.String("inference_server", inferenceServerName))

	// Unload old model from inference server
	a.Logger.Info("Unloading old model from inference server", zap.String("old_model", currentModel))
	if err := a.Gateway.UnloadModel(ctx, a.Logger, currentModel, inferenceServerName, resource.Namespace, v2pb.BACKEND_TYPE_TRITON); err != nil {
		a.Logger.Error("Failed to unload old model from inference server", zap.String("model", currentModel), zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "ModelUnloadingFailed", fmt.Sprintf("Failed to unload old model %s from inference server: %v", currentModel, err)), nil
	}

	a.Logger.Info("Successfully unloaded old model from inference server",
		zap.String("old_model", currentModel),
		zap.String("new_model", desiredModel))

	return conditionUtils.GenerateTrueCondition(condition), nil
}
