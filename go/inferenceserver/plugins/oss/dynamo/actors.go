package dynamo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ValidationActor validates Dynamo-specific configuration
type ValidationActor struct {
	gateway gateways.Gateway
}

func NewValidationActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &ValidationActor{gateway: gateway}
}

func (a *ValidationActor) GetType() string {
	return "DynamoValidation"
}

func (a *ValidationActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Dynamo validation condition")
	
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_DYNAMO {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidBackendType",
			Message: fmt.Sprintf("invalid backend type for Dynamo plugin: %v", resource.Spec.BackendType),
		}, nil
	}
	
	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Dynamo configuration is valid",
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Dynamo validation action")
	
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_DYNAMO {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "InvalidBackendType"
		condition.Message = fmt.Sprintf("invalid backend type for Dynamo plugin: %v", resource.Spec.BackendType)
		return fmt.Errorf("invalid backend type")
	}
	
	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "ValidationSucceeded"
	condition.Message = "Dynamo configuration is valid"
	return nil
}

// PlatformDependenciesActor ensures NATS and ETCD are available
type PlatformDependenciesActor struct {
	gateway gateways.Gateway
}

func NewPlatformDependenciesActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &PlatformDependenciesActor{gateway: gateway}
}

func (a *PlatformDependenciesActor) GetType() string {
	return "DynamoPlatformDependencies"
}

func (a *PlatformDependenciesActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Dynamo platform dependencies condition")
	
	// For now, assume dependencies are available
	// In a real implementation, this would check NATS and ETCD availability
	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "DependenciesReady",
		Message: "Platform dependencies are available",
	}, nil
}

func (a *PlatformDependenciesActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Dynamo platform dependencies action")
	
	// Platform dependencies are handled by the gateway's Dynamo backend
	// This ensures NATS and ETCD are deployed
	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "DependenciesReady"
	condition.Message = "Platform dependencies are available"
	return nil
}

// ResourceCreationActor creates Dynamo infrastructure
type ResourceCreationActor struct {
	gateway gateways.Gateway
}

func NewResourceCreationActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &ResourceCreationActor{gateway: gateway}
}

func (a *ResourceCreationActor) GetType() string {
	return "DynamoResourceCreation"
}

func (a *ResourceCreationActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Dynamo infrastructure condition")
	
	statusResp, err := a.gateway.GetInfrastructureStatus(ctx, logger, gateways.InfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	
	if err != nil {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureCheckFailed",
			Message: "Failed to check infrastructure status",
		}, nil
	}
	
	status := apipb.CONDITION_STATUS_FALSE
	reason := "InfrastructureCreating"
	message := "Infrastructure is being created"
	
	if statusResp.State == v2pb.INFERENCE_SERVER_STATE_SERVING {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "InfrastructureReady"
		message = "Infrastructure is ready"
	}
	
	return apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *ResourceCreationActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Dynamo infrastructure creation")
	
	resources := gateways.ResourceSpec{
		CPU:      "8",
		Memory:   "16Gi",
		GPU:      2,
		Replicas: 1,
	}
	
	_, err := a.gateway.CreateInfrastructure(ctx, logger, gateways.InfrastructureRequest{
		InferenceServer: resource,
		BackendType:     resource.Spec.BackendType,
		Namespace:       resource.Namespace,
		Resources:       resources,
	})
	
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "InfrastructureCreationFailed"
		condition.Message = fmt.Sprintf("Failed to create infrastructure: %v", err)
		return err
	}
	
	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "InfrastructureCreationInitiated"
	condition.Message = "Infrastructure creation initiated successfully"
	return nil
}

// HealthCheckActor checks Dynamo server health
type HealthCheckActor struct {
	gateway gateways.Gateway
}

func NewHealthCheckActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &HealthCheckActor{gateway: gateway}
}

func (a *HealthCheckActor) GetType() string {
	return "DynamoHealthCheck"
}

func (a *HealthCheckActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Dynamo health condition")
	
	healthy, err := a.gateway.IsHealthy(ctx, logger, resource.Name, resource.Spec.BackendType)
	
	status := apipb.CONDITION_STATUS_FALSE
	reason := "HealthCheckFailed"
	message := "Server is not healthy"
	
	if err == nil && healthy {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "HealthCheckSucceeded"
		message = "Server is healthy"
	} else if err != nil {
		message = fmt.Sprintf("Health check error: %v", err)
	}
	
	return apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *HealthCheckActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Dynamo health check action")
	
	healthy, err := a.gateway.IsHealthy(ctx, logger, resource.Name, resource.Spec.BackendType)
	
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "HealthCheckError"
		condition.Message = fmt.Sprintf("Health check error: %v", err)
		return err
	}
	
	if healthy {
		condition.Status = apipb.CONDITION_STATUS_TRUE
		condition.Reason = "HealthCheckSucceeded"
		condition.Message = "Server is healthy"
	} else {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "HealthCheckFailed"
		condition.Message = "Server is not healthy"
		return fmt.Errorf("server is not healthy")
	}
	
	return nil
}

// CleanupActor cleans up Dynamo infrastructure
type CleanupActor struct {
	gateway gateways.Gateway
}

func NewCleanupActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &CleanupActor{gateway: gateway}
}

func (a *CleanupActor) GetType() string {
	return "DynamoCleanup"
}

func (a *CleanupActor) Retrieve(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Dynamo cleanup condition")
	
	_, err := a.gateway.GetInfrastructureStatus(ctx, logger, gateways.InfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	
	status := apipb.CONDITION_STATUS_TRUE
	reason := "CleanupCompleted"
	message := "Infrastructure cleanup completed"
	
	if err == nil {
		status = apipb.CONDITION_STATUS_FALSE
		reason = "CleanupInProgress"
		message = "Infrastructure cleanup in progress"
	}
	
	return apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, logger logr.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Dynamo infrastructure cleanup")
	
	err := a.gateway.DeleteInfrastructure(ctx, logger, gateways.InfrastructureDeleteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "CleanupFailed"
		condition.Message = fmt.Sprintf("Failed to cleanup infrastructure: %v", err)
		return err
	}
	
	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "CleanupInitiated"
	condition.Message = "Infrastructure cleanup initiated successfully"
	return nil
}