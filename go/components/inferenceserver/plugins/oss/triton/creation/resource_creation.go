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

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ResourceCreationActor{}

// ResourceCreationActor provisions Kubernetes resources for Triton inference servers.
type ResourceCreationActor struct {
	backend backends.Backend
	logger  *zap.Logger
}

// NewResourceCreationActor creates a condition actor for Triton infrastructure provisioning.
func NewResourceCreationActor(backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ResourceCreationActor{
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for resource creation.
func (a *ResourceCreationActor) GetType() string {
	return common.TritonResourceCreationConditionType
}

// Retrieve checks if Kubernetes infrastructure exists and is ready.
func (a *ResourceCreationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton infrastructure condition")

	// Check if infrastructure exists
	status, err := a.backend.GetInfrastructureStatus(ctx, a.logger, resource.Name, resource.Namespace)
	if err != nil {
		a.logger.Error("Failed to check infrastructure status",
			zap.Error(err),
			zap.String("operation", "get_infrastructure_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureCheckFailed",
			Message: fmt.Sprintf("Failed to check infrastructure status: %v", err),
		}, nil
	}

	if status.State == v2pb.INFERENCE_SERVER_STATE_SERVING {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "InfrastructureReady",
			Message: "Infrastructure is ready",
		}, nil
	} else if status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
		// Infrastructure doesn't exist or is incomplete, needs to be created
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureNotFound",
			Message: "Infrastructure needs to be created",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "InfrastructureCreating",
		Message: "Infrastructure is being created",
	}, nil
}

// Run creates the Kubernetes deployment, service, and related resources for Triton.
func (a *ResourceCreationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton infrastructure creation")

	_, err := a.backend.CreateInfrastructure(ctx, a.logger, resource)
	if err != nil {
		a.logger.Error("Failed to create infrastructure",
			zap.Error(err),
			zap.String("operation", "create_infrastructure"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureCreationFailed",
			Message: fmt.Sprintf("Failed to create infrastructure: %v", err),
		}, err
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "InfrastructureCreationInitiated",
		Message: "Infrastructure creation initiated successfully",
	}, nil
}
