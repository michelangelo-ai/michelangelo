package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &HealthCheckActor{}

// HealthCheckActor verifies inference server health by polling backend health endpoints.
type HealthCheckActor struct {
	logger        *zap.Logger
	clientFactory clientfactory.ClientFactory
	client        client.Client
	registry      *backends.Registry
}

// NewHealthCheckActor creates a condition actor for inference server health verification.
func NewHealthCheckActor(logger *zap.Logger, client client.Client, clientFactory clientfactory.ClientFactory, registry *backends.Registry) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &HealthCheckActor{
		logger:        logger,
		client:        client,
		clientFactory: clientFactory,
		registry:      registry,
	}
}

// GetType returns the condition type identifier for health checks.
func (a *HealthCheckActor) GetType() string {
	return common.HealthCheckConditionType
}

// Retrieve checks the current health status of the inference server.
func (a *HealthCheckActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server health condition")

	// check if the server is healthy in all target clusters
	targetClusterClients := common.GetClusterClients(ctx, a.logger, resource, a.clientFactory, a.client)
	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}
	for clusterId, client := range targetClusterClients {
		healthy, err := backend.IsHealthy(ctx, a.logger, client, resource.Name, resource.Namespace)
		if err != nil {
			a.logger.Error("Failed to check health",
				zap.Error(err),
				zap.String("operation", "health_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name),
				zap.String("cluster", clusterId))
			return conditionUtils.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Health check error: %v", err)), nil
		}
		if !healthy {
			return conditionUtils.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Server is not healthy in cluster %s", clusterId)), nil
		}
	}
	return conditionUtils.GenerateTrueCondition(condition), nil
}

func (a *HealthCheckActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	return condition, nil
}
