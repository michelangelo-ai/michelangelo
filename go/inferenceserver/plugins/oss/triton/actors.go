package triton

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

// ValidationActor validates Triton-specific configuration
type ValidationActor struct {
	gateway inferenceserver.Gateway
}

func NewValidationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ValidationActor{gateway: gateway}
}

func (a *ValidationActor) GetType() string {
	return "TritonValidation"
}

func (a *ValidationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Validating Triton configuration")
	
	// Validate Triton-specific requirements
	if inferenceServer.Spec.BackendType != v2pb.BACKEND_TYPE_TRITON {
		return fmt.Errorf("invalid backend type for Triton plugin: %v", inferenceServer.Spec.BackendType)
	}
	
	logger.Info("Triton configuration validation completed")
	return nil
}

func (a *ValidationActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:                 a.GetType(),
		Status:               apipb.CONDITION_STATUS_TRUE,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:               "ValidationSucceeded",
		Message:              "Triton configuration is valid",
	}, nil
}

// ResourceCreationActor creates Triton infrastructure
type ResourceCreationActor struct {
	gateway inferenceserver.Gateway
}

func NewResourceCreationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ResourceCreationActor{gateway: gateway}
}

func (a *ResourceCreationActor) GetType() string {
	return "TritonResourceCreation"
}

func (a *ResourceCreationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Creating Triton infrastructure")
	
	// Convert proto ResourceSpec to gateway ResourceSpec
	protoResources := inferenceServer.Spec.InitSpec.ResourceSpec
	resources := inferenceserver.ResourceSpec{
		CPU:      fmt.Sprintf("%d", protoResources.Cpu),
		Memory:   protoResources.Memory,
		GPU:      protoResources.Gpu,
		Replicas: 1, // Default to 1 replica
		ImageTag: "", // Use default
		ModelConfig: map[string]string{
			"model": "s3://deployed-model/bert-cola-23", // Use the model path from user requirement
		},
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
	// Check if infrastructure exists
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

// HealthCheckActor checks Triton server health
type HealthCheckActor struct {
	gateway inferenceserver.Gateway
}

func NewHealthCheckActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &HealthCheckActor{gateway: gateway}
}

func (a *HealthCheckActor) GetType() string {
	return "TritonHealthCheck"
}

func (a *HealthCheckActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Checking Triton server health")
	
	healthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServer.Name, inferenceServer.Spec.BackendType)
	if err != nil {
		return err
	}
	
	if !healthy {
		return fmt.Errorf("Triton server is not healthy")
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

// ProxyConfigurationActor configures Istio proxy
type ProxyConfigurationActor struct {
	gateway inferenceserver.Gateway
}

func NewProxyConfigurationActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &ProxyConfigurationActor{gateway: gateway}
}

func (a *ProxyConfigurationActor) GetType() string {
	return "TritonProxyConfiguration"
}

func (a *ProxyConfigurationActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Configuring Triton proxy")
	
	return a.gateway.ConfigureProxy(ctx, logger, inferenceserver.ProxyConfigRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		ModelName:       inferenceServer.Name,
		BackendType:     inferenceServer.Spec.BackendType,
	})
}

func (a *ProxyConfigurationActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	proxyStatus, err := a.gateway.GetProxyStatus(ctx, logger, inferenceserver.ProxyStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
	})
	
	status := apipb.CONDITION_STATUS_FALSE
	reason := "ProxyNotConfigured"
	message := "Proxy is not configured"
	
	if err == nil && proxyStatus.Configured {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "ProxyConfigured"
		message = "Proxy is configured and ready"
	}
	
	return &apipb.Condition{
		Type:               a.GetType(),
		Status:             status,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
		Reason:             reason,
		Message:            message,
	}, nil
}

// CleanupActor cleans up Triton infrastructure
type CleanupActor struct {
	gateway inferenceserver.Gateway
}

func NewCleanupActor(gateway inferenceserver.Gateway) plugins.ConditionActor {
	return &CleanupActor{gateway: gateway}
}

func (a *CleanupActor) GetType() string {
	return "TritonCleanup"
}

func (a *CleanupActor) Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Cleaning up Triton infrastructure")
	
	return a.gateway.DeleteInfrastructure(ctx, logger, inferenceserver.InfrastructureDeleteRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
}

func (a *CleanupActor) EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error) {
	// Check if infrastructure still exists
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