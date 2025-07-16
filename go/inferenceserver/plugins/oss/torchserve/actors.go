package torchserve

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TorchServeValidationActor validates TorchServe inference server configuration
type TorchServeValidationActor struct {
	gateway gateways.Gateway
}

func (a *TorchServeValidationActor) GetType() string {
	return "TorchServeValidation"
}

func (a *TorchServeValidationActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	// Validate backend type
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_TORCHSERVE {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidBackendType",
			Message: fmt.Sprintf("invalid backend type for TorchServe plugin: %v", resource.Spec.BackendType),
		}, nil
	}

	// Validate resource requirements
	if resource.Spec.InitSpec == nil || resource.Spec.InitSpec.ResourceSpec == nil {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "MissingResourceSpec",
			Message: "resource specification is required for TorchServe",
		}, nil
	}

	// TorchServe-specific validations
	resourceSpec := resource.Spec.InitSpec.ResourceSpec
	if resourceSpec.Cpu < 1 {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InsufficientCPU",
			Message: "TorchServe requires at least 1 CPU core",
		}, nil
	}

	if resourceSpec.Memory == "" {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "MissingMemory",
			Message: "Memory specification is required for TorchServe",
		}, nil
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "TorchServe configuration is valid",
	}, nil
}

func (a *TorchServeValidationActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running TorchServe validation", "server", resource.Name)
	
	// Update server state to indicate validation is in progress
	resource.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	// Note: InferenceServerStatus doesn't have a Message field in this version
	
	logger.Info("TorchServe validation completed", "server", resource.Name)
	return nil
}

// TorchServeResourceCreationActor creates TorchServe infrastructure resources
type TorchServeResourceCreationActor struct {
	gateway gateways.Gateway
}

func (a *TorchServeResourceCreationActor) GetType() string {
	return "TorchServeResourceCreation"
}

func (a *TorchServeResourceCreationActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	// Check if infrastructure was created successfully
	request := gateways.InfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TORCHSERVE,
	}

	status, err := a.gateway.GetInfrastructureStatus(ctx, logger, request)
	if err != nil {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "StatusCheckFailed",
			Message: fmt.Sprintf("Failed to check infrastructure status: %v", err),
		}, nil
	}

	if status.Ready {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ResourcesCreated",
			Message: "TorchServe infrastructure resources created successfully",
		}, nil
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ResourcesNotReady",
		Message: "TorchServe infrastructure resources are not ready yet",
	}, nil
}

func (a *TorchServeResourceCreationActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Creating TorchServe infrastructure", "server", resource.Name)

	// Create infrastructure using gateway
	request := gateways.InfrastructureRequest{
		InferenceServer: resource,
		BackendType:     v2pb.BACKEND_TYPE_TORCHSERVE,
		Namespace:       resource.Namespace,
		Resources: gateways.ResourceSpec{
			CPU:      fmt.Sprintf("%d", resource.Spec.InitSpec.ResourceSpec.Cpu),
			Memory:   resource.Spec.InitSpec.ResourceSpec.Memory,
			GPU:      resource.Spec.InitSpec.ResourceSpec.Gpu,
			Replicas: resource.Spec.InitSpec.NumInstances,
			ImageTag: resource.Spec.InitSpec.ServingSpec.Version,
		},
	}

	response, err := a.gateway.CreateInfrastructure(ctx, logger, request)
	if err != nil {
		resource.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
		// Update state without message field
		return nil
	}

	// Update server status
	resource.Status.State = response.State
	// Update state without message field

	logger.Info("TorchServe infrastructure creation initiated", "server", resource.Name)
	return nil
}

// TorchServeHealthCheckActor monitors TorchServe server health
type TorchServeHealthCheckActor struct {
	gateway gateways.Gateway
}

func (a *TorchServeHealthCheckActor) GetType() string {
	return "TorchServeHealthCheck"
}

func (a *TorchServeHealthCheckActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	// Check TorchServe health
	healthy, err := a.gateway.IsHealthy(ctx, logger, resource.Name, v2pb.BACKEND_TYPE_TORCHSERVE)
	if err != nil {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "HealthCheckFailed",
			Message: fmt.Sprintf("Health check failed: %v", err),
		}, nil
	}

	if healthy {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "HealthCheckSucceeded",
			Message: "Server is healthy",
		}, nil
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ServerUnhealthy",
		Message: "Server is not healthy",
	}, nil
}

func (a *TorchServeHealthCheckActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running TorchServe health check", "server", resource.Name)

	// The health check is performed in Retrieve method
	// Here we can update any additional status if needed
	
	logger.Info("TorchServe health check completed", "server", resource.Name)
	return nil
}

// TorchServeCleanupActor handles TorchServe resource cleanup
type TorchServeCleanupActor struct {
	gateway gateways.Gateway
}

func (a *TorchServeCleanupActor) GetType() string {
	return "TorchServeCleanup"
}

func (a *TorchServeCleanupActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	// Check if resource is marked for deletion
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CleanupRequired",
			Message: "Resource cleanup is required",
		}, nil
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CleanupNotNeeded",
		Message: "Resource cleanup not needed",
	}, nil
}

func (a *TorchServeCleanupActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running TorchServe cleanup", "server", resource.Name)

	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		// Delete infrastructure
		request := gateways.InfrastructureDeleteRequest{
			InferenceServer: resource.Name,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TORCHSERVE,
		}

		if err := a.gateway.DeleteInfrastructure(ctx, logger, request); err != nil {
			logger.Error(err, "Failed to delete TorchServe infrastructure")
			return err
		}

		logger.Info("TorchServe infrastructure cleaned up", "server", resource.Name)
	}

	return nil
}