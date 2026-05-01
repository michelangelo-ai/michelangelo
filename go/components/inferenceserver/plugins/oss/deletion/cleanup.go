package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor removes all Kubernetes resources associated with a Triton inference server.
type CleanupActor struct {
	logger        *zap.Logger
	client        client.Client
	clientFactory clientfactory.ClientFactory
	backend       backends.Backend
}

// NewCleanupActor creates a condition actor for inference server cleanup during deletion.
func NewCleanupActor(logger *zap.Logger, client client.Client, clientFactory clientfactory.ClientFactory, backend backends.Backend) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		logger:        logger,
		client:        client,
		clientFactory: clientFactory,
		backend:       backend,
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
	targetClusterClients := common.GetClusterClients(ctx, a.logger, resource, a.clientFactory, a.client)
	for clusterId, client := range targetClusterClients {
		status, err := a.backend.GetServerStatus(ctx, a.logger, client, resource.Name, resource.Namespace)
		if err != nil {
			return conditionsUtils.GenerateFalseCondition(condition, "CannotCheckServerStatus", "Failed to check server status"), nil
		}
		if status.State != v2pb.INFERENCE_SERVER_STATE_DELETED {
			return conditionsUtils.GenerateFalseCondition(condition, "ServerNotDeleted", fmt.Sprintf("Server %s is not deleted in cluster %s", resource.Name, clusterId)), nil
		}
	}
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// Run deletes the deployment, service, ConfigMaps for the inference server.
func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton inference server cleanup with ConfigMap cleanup")

	// todo: ghosharitra: we may need to create and delete model configs within the plugins now.
	// Delete inference server in all target clusters
	a.logger.Info("Cleaning up inference server", zap.String("inferenceServer", resource.Name))
	targetClusterClients := common.GetClusterClients(ctx, a.logger, resource, a.clientFactory, a.client)
	for clusterId, client := range targetClusterClients {
		err := a.backend.DeleteServer(ctx, a.logger, client, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Failed to delete inference server",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterId))
			return conditionsUtils.GenerateFalseCondition(condition, "ServerCleanupFailed", fmt.Sprintf("failed to cleanup inference server in cluster %s: %v", clusterId, err)), nil
		}
	}

	a.logger.Info("Triton inference server cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return conditionsUtils.GenerateTrueCondition(condition), nil
}
