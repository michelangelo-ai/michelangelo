package llmd

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

// ValidationActor validates LLMD-specific configuration
type ValidationActor struct {
	gateway inferenceserver.Gateway
}

func NewValidationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ValidationActor{gateway: gateway}
}

func (a *ValidationActor) GetType() string {
	return "LLMDValidation"
}

func (a *ValidationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Validating LLMD configuration")
	
	if inferenceServer.Spec.BackendType != v2pb.BACKEND_TYPE_LLM_D {
		return fmt.Errorf("invalid backend type for LLMD plugin: %v", inferenceServer.Spec.BackendType)
	}
	
	return nil
}

func (a *ValidationActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             apipb.CONDITION_STATUS_TRUE,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             "ValidationSucceeded",
		Message:            "LLMD configuration is valid",
	}, nil
}

// ResourceCreationActor creates LLMD infrastructure
type ResourceCreationActor struct {
	gateway inferenceserver.Gateway
}

func NewResourceCreationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ResourceCreationActor{gateway: gateway}
}

func (a *ResourceCreationActor) GetType() string {
	return "LLMDResourceCreation"
}

func (a *ResourceCreationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Creating LLMD infrastructure")
	
	resources := inferenceserver.ResourceSpec{
		CPU:      "4",
		Memory:   "8Gi",
		GPU:      1,
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

// HealthCheckActor checks LLMD server health
type HealthCheckActor struct {
	gateway inferenceserver.Gateway
}

func NewHealthCheckActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &HealthCheckActor{gateway: gateway}
}

func (a *HealthCheckActor) GetType() string {
	return "LLMDHealthCheck"
}

func (a *HealthCheckActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Checking LLMD server health")
	
	healthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServer.Name, inferenceServer.Spec.BackendType)
	if err != nil {
		return err
	}
	
	if !healthy {
		return fmt.Errorf("LLMD server is not healthy")
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

// CleanupActor cleans up LLMD infrastructure
type CleanupActor struct {
	gateway inferenceserver.Gateway
}

func NewCleanupActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &CleanupActor{gateway: gateway}
}

func (a *CleanupActor) GetType() string {
	return "LLMDCleanup"
}

func (a *CleanupActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Cleaning up LLMD infrastructure")
	
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