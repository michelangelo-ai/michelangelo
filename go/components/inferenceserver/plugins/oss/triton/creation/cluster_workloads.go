package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ClusterWorkloadsActor{}

// ClusterWorkloadsActor provisions Kubernetes resources for Triton inference servers.
type ClusterWorkloadsActor struct {
	backend backends.Backend
	logger  *zap.Logger
}

// NewClusterWorkloadsActor creates a condition actor for Triton server provisioning.
func NewClusterWorkloadsActor(backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ClusterWorkloadsActor{
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for resource creation.
func (a *ClusterWorkloadsActor) GetType() string {
	return common.TritonClusterWorkloadsConditionType
}

// Retrieve checks if Kubernetes infrastructure for all target clusters exists and is ready.
func (a *ClusterWorkloadsActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton server condition")

	for _, targetCluster := range common.GetTargetClusters(resource.Spec.GetDeploymentStrategy()) {
		status, err := a.backend.GetServerStatus(ctx, resource.Name, resource.Namespace, targetCluster)
		if err != nil {
			a.logger.Error("Failed to check server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", common.GenerateClusterDisplayName(targetCluster)))
			return conditionsUtils.GenerateFalseCondition(condition, "ClusterCheckFailed",
				fmt.Sprintf("Failed to check cluster %s status", common.GenerateClusterDisplayName(targetCluster))), nil
		}
		if status.ClusterState != v2pb.CLUSTER_STATE_READY {
			return conditionsUtils.GenerateUnknownCondition(condition, "ClusterNotReady",
				fmt.Sprintf("Cluster %s is in state %s", common.GenerateClusterDisplayName(targetCluster), status.ClusterState)), nil
		}
	}
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// Run ensures that the infrastructure for all target clusters exists and is ready.
func (a *ClusterWorkloadsActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton server infrastructure creation for all target clusters")

	constraints := backends.ResourceConstraints{
		Cpu:      resource.Spec.InitSpec.ResourceSpec.Cpu,
		Memory:   resource.Spec.InitSpec.ResourceSpec.Memory,
		Gpu:      resource.Spec.InitSpec.ResourceSpec.Gpu,
		Replicas: resource.Spec.InitSpec.NumInstances,
	}

	for _, targetCluster := range common.GetTargetClusters(resource.Spec.GetDeploymentStrategy()) {
		_, err := a.backend.CreateServer(ctx, resource.Name, resource.Namespace, constraints, targetCluster)
		if err != nil {
			a.logger.Error("Failed to create server",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", common.GenerateClusterDisplayName(targetCluster)))
			return conditionsUtils.GenerateFalseCondition(condition, "ClusterCreationFailed",
				fmt.Sprintf("Failed to create server in cluster %s: %v", common.GenerateClusterDisplayName(targetCluster), err)), nil
		}
	}
	return conditionsUtils.GenerateUnknownCondition(condition, "ClusterCreationInitiated",
		"server creation initiated in all target clusters, waiting for resources to be ready"), nil
}
