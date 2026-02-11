package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor removes all Kubernetes resources associated with an inference server.
type CleanupActor struct {
	client              client.Client
	registry            *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

// NewCleanupActor creates a condition actor for inference server cleanup during deletion.
func NewCleanupActor(client client.Client, registry *backends.Registry, modelConfigProvider modelconfig.ModelConfigProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		client:              client,
		registry:            registry,
		modelConfigProvider: modelConfigProvider,
		logger:              logger,
	}
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.CleanupConditionType
}

// Retrieve checks if all inference server has been successfully deleted.
func (a *CleanupActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server cleanup condition")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		// If backend is not found, then consider cleanup is complete
		return conditionsUtil.GenerateTrueCondition(condition), nil
	}

	// Check if inference server still exists
	_, err = backend.GetServerStatus(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err == nil {
		return conditionsUtil.GenerateFalseCondition(condition, "CleanupInProgress", "Inference server cleanup in progress"), nil
	}

	return conditionsUtil.GenerateTrueCondition(condition), nil
}

// Run deletes the deployment, service, ConfigMaps for the inference server.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running inference server cleanup with ConfigMap cleanup")

	// Delete Model Config first
	a.logger.Info("Cleaning up Model Config for inference server", zap.String("inferenceServer", resource.Name))

	// Clean up model-config
	if err := a.modelConfigProvider.DeleteModelConfig(ctx, a.logger, a.client, resource.Name, resource.Namespace); err != nil {
		a.logger.Error("Failed to delete Model Config",
			zap.Error(err),
			zap.String("operation", "delete_modelconfig"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
		)
	} else {
		a.logger.Info("Successfully deleted Model Config for inference server", zap.String("inferenceServer", resource.Name))
	}

	// Get backend from registry
	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		// If backend is not found, then consider cleanup is complete
		return conditionsUtil.GenerateTrueCondition(condition), nil
	}

	// Delete inference server
	a.logger.Info("Cleaning up inference server", zap.String("inferenceServer", resource.Name))
	err = backend.DeleteServer(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to delete inference server",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return conditionsUtil.GenerateFalseCondition(condition, "ServerCleanupFailed", fmt.Sprintf("Failed to cleanup inference server: %v", err)), fmt.Errorf("delete inference server %s/%s: %w", resource.Namespace, resource.Name, err)
	}

	a.logger.Info("Inference server cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return conditionsUtil.GenerateTrueCondition(condition), nil
}
