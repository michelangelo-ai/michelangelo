package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
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

	// todo: ghosharitra: update this so that it checks all the cluster targets
	healthy, err := a.backend.IsHealthy(ctx, a.logger, resource)

	if err == nil && healthy {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "HealthCheckSucceeded",
			Message: "Server is healthy",
		}, nil
	} else if err != nil {
		a.logger.Error("Health check failed",
			zap.Error(err),
			zap.String("operation", "health_check"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HealthCheckFailed",
			Message: fmt.Sprintf("Health check error: %v", err),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "HealthCheckFailed",
		Message: "Server is not healthy",
	}, nil
}

// todo: ghosharitra: revise this later
// Run returns an unknown condition to trigger re-checking on next reconciliation.
// Health check failures are expected during startup as pods are still coming up.
func (a *HealthCheckActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is called when Retrieve() returns non-TRUE status.
	// Return UNKNOWN to keep reconciling and wait for pods to become healthy.
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_UNKNOWN,
		Reason:  "HealthCheckPending",
		Message: "Waiting for server to become healthy",
	}, nil
}
