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

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &HealthCheckActor{}

// HealthCheckActor verifies inference server health by polling backend health endpoints.
type HealthCheckActor struct {
	backend backends.Backend
	logger  *zap.Logger
}

// NewHealthCheckActor creates a condition actor for Triton health verification.
func NewHealthCheckActor(backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &HealthCheckActor{
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for health checks.
func (a *HealthCheckActor) GetType() string {
	return common.TritonHealthCheckConditionType
}

// Retrieve checks the current health status of the Triton server.
func (a *HealthCheckActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton health condition")

	// todo: ghosharitra: revise this
	for _, targetCluster := range resource.Spec.ClusterTargets {
		healthy, err := a.backend.IsHealthy(ctx, resource.Name, resource.Namespace, targetCluster)
		if err != nil {
			a.logger.Error("Failed to check health",
				zap.Error(err),
				zap.String("operation", "health_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("inferenceServer", resource.Name))
			return conditionsUtils.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Health check error: %v", err)), nil
		}
		if !healthy {
			return conditionsUtils.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Server is not healthy in cluster %s", targetCluster.ClusterId)), nil
		}
	}
	return conditionsUtils.GenerateTrueCondition(condition), nil
}

// todo: ghosharitra: revise this later
func (a *HealthCheckActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	return condition, nil
}
