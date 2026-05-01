package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	conditionsUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &BackendProvisionActor{}

// BackendProvisionActor provisions Kubernetes resources for inference servers.
type BackendProvisionActor struct {
	logger        *zap.Logger
	client        client.Client
	clientFactory clientfactory.ClientFactory
	registry      *backends.Registry
}

// NewBackendProvisionActor creates a condition actor for inference server provisioning.
func NewBackendProvisionActor(logger *zap.Logger, client client.Client, clientFactory clientfactory.ClientFactory, registry *backends.Registry) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &BackendProvisionActor{
		logger:        logger,
		client:        client,
		clientFactory: clientFactory,
		registry:      registry,
	}
}

// GetType returns the condition type identifier for resource creation.
func (a *BackendProvisionActor) GetType() string {
	return common.BackendProvisionConditionType
}

// Retrieve checks if Kubernetes infrastructure for all target clusters exists and is ready.
func (a *BackendProvisionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server backend provision condition")

	targetClusterClients := common.GetClusterClients(ctx, a.logger, resource, a.clientFactory, a.client)
	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}
	for clusterId, client := range targetClusterClients {
		status, err := backend.GetServerStatus(ctx, a.logger, client, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Failed to check server status",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterId))
			return conditionsUtils.GenerateFalseCondition(condition, "ClusterCheckFailed",
				fmt.Sprintf("Failed to check cluster %s status", clusterId)), nil
		}
		if status.State != v2pb.INFERENCE_SERVER_STATE_SERVING {
			return conditionsUtils.GenerateUnknownCondition(condition, "ClusterNotReady",
				fmt.Sprintf("Cluster %s is in state %s", clusterId, status.State)), nil
		}
	}
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// Run ensures that the infrastructure for all target clusters exists and is ready.
func (a *BackendProvisionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running inference server backend provision for all target clusters")

	targetClusterClients := common.GetClusterClients(ctx, a.logger, resource, a.clientFactory, a.client)
	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}
	for clusterId, client := range targetClusterClients {
		_, err := backend.CreateServer(ctx, a.logger, client, resource)
		if err != nil {
			a.logger.Error("Failed to create server",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterId))
			return conditionsUtils.GenerateFalseCondition(condition, "ClusterCreationFailed",
				fmt.Sprintf("Failed to create server in cluster %s: %v", clusterId, err)), nil
		}
	}
	return conditionsUtils.GenerateUnknownCondition(condition, "ClusterCreationInitiated",
		"server creation initiated in all target clusters, waiting for resources to be ready"), nil
}
