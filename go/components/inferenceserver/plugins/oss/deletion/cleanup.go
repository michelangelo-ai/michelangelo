package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
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
	backend             backends.Backend
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

// NewCleanupActor creates a condition actor for inference server cleanup during deletion.
func NewCleanupActor(client client.Client, backend backends.Backend, modelConfigProvider modelconfig.ModelConfigProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		client:              client,
		backend:             backend,
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

	// Check if inference server still exists
	_, err := a.backend.GetServerStatus(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "CleanupInProgress",
			Message: "Inference server cleanup in progress",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Inference server cleanup completed",
	}, nil
}

// Run deletes the deployment, service, ConfigMaps for the inference server.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running inference server cleanup with ConfigMap cleanup")

	// Delete ConfigMaps first
	a.logger.Info("Cleaning up ConfigMaps for inference server", zap.String("inferenceServer", resource.Name))

	// Clean up model-config ConfigMap
	modelConfigMapName := fmt.Sprintf("%s-model-config", resource.Name)
	if err := a.modelConfigProvider.DeleteModelConfig(ctx, a.logger, a.client, resource.Name, resource.Namespace); err != nil {
		a.logger.Error("Failed to delete model ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_modelconfig"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("configMap", modelConfigMapName))
		// Don't fail the whole cleanup for ConfigMap errors, but log them
	} else {
		a.logger.Info("Successfully deleted model ConfigMap", zap.String("configMap", modelConfigMapName))
	}

	// Delete inference server
	a.logger.Info("Cleaning up inference server", zap.String("inferenceServer", resource.Name))
	err := a.backend.DeleteServer(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to delete inference server",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ServerCleanupFailed",
			Message: fmt.Sprintf("Failed to cleanup inference server: %v", err),
		}, fmt.Errorf("delete inference server %s/%s: %w", resource.Namespace, resource.Name, err)
	}

	a.logger.Info("Inference server cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupInitiated",
		Message: "Inference server, model ConfigMap cleanup initiated successfully",
	}, nil
}
