package strategies

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelCleanupActor{}

// ModelCleanupActor unloads previous model versions from inference servers after successful rollout.
type ModelCleanupActor struct {
	Client              client.Client
	HTTPClient          *http.Client
	BackendRegistry     *backends.Registry
	ModelConfigProvider modelconfig.ModelConfigProvider
	Logger              *zap.Logger
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
	serverBackend, err := a.BackendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "CleanupPending", fmt.Sprintf("Failed to get backend for inference server %s: %v", inferenceServerName, err)), err
	}
	ready, err := serverBackend.CheckModelStatus(ctx, a.Logger, a.Client, a.HTTPClient, inferenceServerName, resource.Namespace, currentModel)
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
	a.Logger.Info("Removing old model from model config", zap.String("old_model", currentModel))
	if err := a.ModelConfigProvider.RemoveModelFromConfig(ctx, a.Logger, a.Client, inferenceServerName, resource.Namespace, currentModel); err != nil {
		a.Logger.Error("Failed to remove old model from model config", zap.String("model", currentModel), zap.Error(err))
		return conditionUtils.GenerateFalseCondition(condition, "ModelRemovalFailed", fmt.Sprintf("Failed to remove old model %s from model config: %v", currentModel, err)), nil
	}
	a.Logger.Info("Successfully removed old model from model config", zap.String("old_model", currentModel))

	return conditionUtils.GenerateTrueCondition(condition), nil
}
