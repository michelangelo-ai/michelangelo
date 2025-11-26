package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ResourceCreationActor{}

// ResourceCreationActor creates Triton infrastructure
type ResourceCreationActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

func NewResourceCreationActor(gateway gateways.Gateway, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ResourceCreationActor{
		gateway: gateway,
		logger:  logger,
	}
}

func (a *ResourceCreationActor) GetType() string {
	return "TritonResourceCreation"
}

func (a *ResourceCreationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton infrastructure condition")

	// Check if infrastructure exists
	statusResp, err := a.gateway.GetInfrastructureStatus(ctx, a.logger, gateways.GetInfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	if err != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureCheckFailed",
			Message: "Failed to check infrastructure status",
		}, nil
	}

	if statusResp.Status.State == v2pb.INFERENCE_SERVER_STATE_SERVING {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "InfrastructureReady",
			Message: "Infrastructure is ready",
		}, nil
	} else if statusResp.Status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
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

func (a *ResourceCreationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton infrastructure creation")

	// Convert proto ResourceSpec to gateway ResourceSpec
	protoResources := resource.Spec.InitSpec.ResourceSpec
	resources := gateways.ResourceSpec{
		CPU:      fmt.Sprintf("%d", protoResources.Cpu),
		Memory:   protoResources.Memory,
		GPU:      protoResources.Gpu,
		Replicas: 1, // Default to 1 replica
	}

	_, err := a.gateway.CreateInfrastructure(ctx, a.logger, gateways.CreateInfrastructureRequest{
		InferenceServer: resource,
		BackendType:     resource.Spec.BackendType,
		Namespace:       resource.Namespace,
		Resources:       resources,
	})
	if err != nil {
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
