package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor removes all Kubernetes resources associated with a Triton inference server.
type CleanupActor struct {
	backend                backends.Backend
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

// NewCleanupActor creates a condition actor for inference server cleanup during deletion.
func NewCleanupActor(backend backends.Backend, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		backend:                backend,
		modelConfigMapProvider: modelConfigMapProvider,
		logger:                 logger,
	}
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.TritonCleanupConditionType
}

// Retrieve checks if all inference server has been successfully deleted.
func (a *CleanupActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton cleanup condition")

	// Check if inference server still exists
	for _, clusterTarget := range resource.Spec.ClusterTargets {
		status, err := a.backend.GetServerStatus(ctx, resource.Name, resource.Namespace, clusterTarget)
		if err != nil {
			return conditionsUtils.GenerateFalseCondition(condition, "CannotCheckServerStatus", "Failed to check server status"), nil
		}
		if status.ClusterState != v2pb.CLUSTER_STATE_INVALID {
			return conditionsUtils.GenerateFalseCondition(condition, "ServerNotDeleted", fmt.Sprintf("Server %s is not deleted in cluster %s", resource.Name, clusterTarget.ClusterId)), nil
		}
	}
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// Run deletes the deployment, service, ConfigMaps for the inference server.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton inference server cleanup with ConfigMap cleanup")

	// Delete inference server in all target clusters
	a.logger.Info("Cleaning up inference server", zap.String("inferenceServer", resource.Name))
	for _, clusterTarget := range resource.Spec.ClusterTargets {
		err := a.backend.DeleteServer(ctx, resource.Name, resource.Namespace, clusterTarget)
		if err != nil {
			a.logger.Error("Failed to delete inference server",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name))
			return conditionsUtils.GenerateFalseCondition(condition, "ServerCleanupFailed", fmt.Sprintf("failed to cleanup inference server in cluster %s: %v", clusterTarget.ClusterId, err)), nil
		}
	}

	a.logger.Info("Triton inference server cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return conditionsUtils.GenerateTrueCondition(condition), nil
}
