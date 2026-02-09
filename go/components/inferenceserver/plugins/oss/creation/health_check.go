package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &HealthCheckActor{}

// HealthCheckActor verifies inference server health by polling backend health endpoints.
type HealthCheckActor struct {
	registry *backends.Registry
	logger   *zap.Logger
	client   client.Client
}

// NewHealthCheckActor creates a condition actor for inference server health verification.
func NewHealthCheckActor(client client.Client, registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &HealthCheckActor{
		client:   client,
		registry: registry,
		logger:   logger,
	}
}

// GetType returns the condition type identifier for health checks.
func (a *HealthCheckActor) GetType() string {
	return common.HealthCheckConditionType
}

// Retrieve checks the current health status of the inference server.
func (a *HealthCheckActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server health condition")

	backend, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "BackendNotFound", fmt.Sprintf("Failed to get backend: %v", err)), nil
	}

	healthy, err := backend.IsHealthy(ctx, a.logger, a.client, resource.Name, resource.Namespace)
	if err == nil && healthy {
		return conditionUtils.GenerateTrueCondition(condition), nil
	} else if err != nil {
		a.logger.Error("Health check failed",
			zap.Error(err),
			zap.String("operation", "health_check"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return conditionUtils.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Health check error: %v", err)), nil
	}

	return conditionUtils.GenerateFalseCondition(condition, "HealthCheckFailed", "Server is not healthy"), nil
}

// Run returns a failed condition since health check failures cannot be automatically remediated.
func (a *HealthCheckActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is only run when Retrieve() fails.
	// If Retrieve() failed, then there's nothing we can do here, simply return the condition.
	return condition, nil
}
