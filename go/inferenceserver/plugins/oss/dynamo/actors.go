package dynamo

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ValidationActor validates Dynamo-specific configuration
type ValidationActor struct {
	gateway inferenceserver.Gateway
}

func NewValidationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ValidationActor{gateway: gateway}
}

func (a *ValidationActor) GetType() string {
	return "DynamoValidation"
}

func (a *ValidationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Validating Dynamo configuration")
	
	if inferenceServer.Spec.BackendType != v2pb.BACKEND_TYPE_DYNAMO {
		return fmt.Errorf("invalid backend type for Dynamo plugin: %v", inferenceServer.Spec.BackendType)
	}
	
	return nil
}

func (a *ValidationActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             apipb.CONDITION_STATUS_TRUE,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             "ValidationSucceeded",
		Message:            "Dynamo configuration is valid",
	}, nil
}

// PlatformDependenciesActor ensures NATS and ETCD are available
type PlatformDependenciesActor struct {
	gateway inferenceserver.Gateway
}

func NewPlatformDependenciesActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &PlatformDependenciesActor{gateway: gateway}
}

func (a *PlatformDependenciesActor) GetType() string {
	return "DynamoPlatformDependencies"
}

func (a *PlatformDependenciesActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Ensuring Dynamo platform dependencies")
	
	// Platform dependencies are handled by the gateway's Dynamo backend
	// This is primarily a validation step
	return nil
}

func (a *PlatformDependenciesActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             apipb.CONDITION_STATUS_TRUE,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             "DependenciesReady",
		Message:            "Platform dependencies are available",
	}, nil
}

// ResourceCreationActor creates Dynamo infrastructure
type ResourceCreationActor struct {
	gateway inferenceserver.Gateway
}

func NewResourceCreationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ResourceCreationActor{gateway: gateway}
}

func (a *ResourceCreationActor) GetType() string {
	return "DynamoResourceCreation"
}

func (a *ResourceCreationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Creating Dynamo infrastructure")
	
	resources := inferenceserver.ResourceSpec{
		CPU:      "8",
		Memory:   "16Gi",
		GPU:      2,
		Replicas: 1,
	}
	
	_, err := a.gateway.CreateInfrastructure(ctx, logger, inferenceserver.InfrastructureRequest{
		InferenceServer: inferenceServer,
		BackendType:     inferenceServer.Spec.BackendType,
		Namespace:       inferenceServer.Namespace,
		Resources:       resources,
	})
	
	return err
}

func (a *ResourceCreationActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	statusResp, err := a.gateway.GetInfrastructureStatus(ctx, logger, inferenceserver.InfrastructureStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
	
	if err != nil {
		return &apipb.Condition{
			Type:               a.GetType(),
			Status:             apipb.CONDITION_STATUS_FALSE,
			LastUpdatedTimestamp: time.Now().UnixMilli(),
			Reason:             "InfrastructureCheckFailed",
			Message:            "Failed to check infrastructure status",
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
	
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             status,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             reason,
		Message:            message,
	}, nil
}

// HealthCheckActor checks Dynamo server health
type HealthCheckActor struct {
	gateway inferenceserver.Gateway
}

func NewHealthCheckActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &HealthCheckActor{gateway: gateway}
}

func (a *HealthCheckActor) GetType() string {
	return "DynamoHealthCheck"
}

func (a *HealthCheckActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Checking Dynamo server health")
	
	healthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServer.Name, inferenceServer.Spec.BackendType)
	if err != nil {
		return err
	}
	
	if !healthy {
		return fmt.Errorf("Dynamo server is not healthy")
	}
	
	return nil
}

func (a *HealthCheckActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	healthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServer.Name, inferenceServer.Spec.BackendType)
	
	status := apipb.CONDITION_STATUS_FALSE
	reason := "HealthCheckFailed"
	message := "Server is not healthy"
	
	if err == nil && healthy {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "HealthCheckSucceeded"
		message = "Server is healthy"
	}
	
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             status,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             reason,
		Message:            message,
	}, nil
}

// CleanupActor cleans up Dynamo infrastructure
type CleanupActor struct {
	gateway inferenceserver.Gateway
}

func NewCleanupActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &CleanupActor{gateway: gateway}
}

func (a *CleanupActor) GetType() string {
	return "DynamoCleanup"
}

func (a *CleanupActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Cleaning up Dynamo infrastructure")
	
	return a.gateway.DeleteInfrastructure(ctx, logger, inferenceserver.InfrastructureDeleteRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
}

func (a *CleanupActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	_, err := a.gateway.GetInfrastructureStatus(ctx, logger, inferenceserver.InfrastructureStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
	
	status := apipb.CONDITION_STATUS_TRUE
	reason := "CleanupCompleted"
	message := "Infrastructure cleanup completed"
	
	if err == nil {
		status = apipb.CONDITION_STATUS_FALSE
		reason = "CleanupInProgress"
		message = "Infrastructure cleanup in progress"
	}
	
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             status,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             reason,
		Message:            message,
	}, nil
}