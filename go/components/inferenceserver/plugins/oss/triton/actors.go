package triton

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ValidationActor validates Triton-specific configuration
type ValidationActor struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
}

func NewValidationActor(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider) plugins.ConditionActor {
	return &ValidationActor{
		gateway:       gateway,
		proxyProvider: proxyProvider,
	}
}

func (a *ValidationActor) GetType() string {
	return "TritonValidation"
}

func (a *ValidationActor) Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Triton validation condition")

	// Validate Triton-specific requirements
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_TRITON {
		return apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidBackendType",
			Message: fmt.Sprintf("invalid backend type for Triton plugin: %v", resource.Spec.BackendType),
		}, nil
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Triton configuration is valid",
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Triton validation action")

	// For validation, there's no corrective action - just update condition status
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_TRITON {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "InvalidBackendType"
		condition.Message = fmt.Sprintf("invalid backend type for Triton plugin: %v", resource.Spec.BackendType)
		return fmt.Errorf("invalid backend type")
	}

	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "ValidationSucceeded"
	condition.Message = "Triton configuration is valid"
	return nil
}

// ResourceCreationActor creates Triton infrastructure
type ResourceCreationActor struct {
	gateway gateways.Gateway
}

func NewResourceCreationActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &ResourceCreationActor{gateway: gateway}
}

func (a *ResourceCreationActor) GetType() string {
	return "TritonResourceCreation"
}

func (a *ResourceCreationActor) Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Triton infrastructure condition")

	// Check if infrastructure exists
	statusResp, err := a.gateway.GetInfrastructureStatus(ctx, logger, gateways.GetInfrastructureStatusRequest{
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

	if statusResp.Status.State == v2pb.INFERENCE_SERVER_STATE_SERVING {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "InfrastructureReady"
		message = "Infrastructure is ready"
	} else if statusResp.Status.State == v2pb.INFERENCE_SERVER_STATE_CREATING {
		// Infrastructure doesn't exist or is incomplete, needs to be created
		status = apipb.CONDITION_STATUS_FALSE
		reason = "InfrastructureNotFound"
		message = "Infrastructure needs to be created"
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *ResourceCreationActor) Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Triton infrastructure creation")

	// Convert proto ResourceSpec to gateway ResourceSpec
	protoResources := resource.Spec.InitSpec.ResourceSpec
	resources := gateways.ResourceSpec{
		CPU:      fmt.Sprintf("%d", protoResources.Cpu),
		Memory:   protoResources.Memory,
		GPU:      protoResources.Gpu,
		Replicas: 1,  // Default to 1 replica
		ImageTag: "", // Use default
		ModelConfig: map[string]string{
			"model": "s3://deploy-models/bert-cola-23", // Use the model path from user requirement
		},
	}

	_, err := a.gateway.CreateInfrastructure(ctx, logger, gateways.CreateInfrastructureRequest{
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

// HealthCheckActor checks Triton server health
type HealthCheckActor struct {
	gateway gateways.Gateway
}

func NewHealthCheckActor(gateway gateways.Gateway) plugins.ConditionActor {
	return &HealthCheckActor{gateway: gateway}
}

func (a *HealthCheckActor) GetType() string {
	return "TritonHealthCheck"
}

func (a *HealthCheckActor) Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Triton health condition")

	healthy, err := a.gateway.IsHealthy(ctx, logger, gateways.HealthCheckRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})

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

func (a *HealthCheckActor) Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Triton health check action")

	// For health checks, there's typically no corrective action
	// Just update the condition based on current health status
	healthy, err := a.gateway.IsHealthy(ctx, logger, gateways.HealthCheckRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
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

// ProxyConfigurationActor configures Istio proxy
type ProxyConfigurationActor struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
}

func NewProxyConfigurationActor(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider) plugins.ConditionActor {
	return &ProxyConfigurationActor{
		gateway:       gateway,
		proxyProvider: proxyProvider,
	}
}

func (a *ProxyConfigurationActor) GetType() string {
	return "TritonProxyConfiguration"
}

func (a *ProxyConfigurationActor) Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Triton proxy configuration condition")

	proxyStatus, err := a.proxyProvider.GetProxyStatus(ctx, logger, proxy.GetProxyStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	})

	status := apipb.CONDITION_STATUS_FALSE
	reason := "ProxyNotConfigured"
	message := "Proxy is not configured"

	if err == nil && proxyStatus.Status.Configured {
		status = apipb.CONDITION_STATUS_TRUE
		reason = "ProxyConfigured"
		message = "Proxy is configured and ready"
	} else if err != nil {
		message = fmt.Sprintf("Failed to check proxy status: %v", err)
	}

	return apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *ProxyConfigurationActor) Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Triton proxy configuration")

	err := a.proxyProvider.EnsureInferenceServerRoute(ctx, logger, proxy.EnsureInferenceServerRouteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		ModelName:       resource.Name,
	})
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "ProxyConfigurationFailed"
		condition.Message = fmt.Sprintf("Failed to configure proxy: %v", err)
		return err
	}

	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "ProxyConfigured"
	condition.Message = "Proxy configured successfully"
	return nil
}

// CleanupActor cleans up Triton infrastructure
type CleanupActor struct {
	gateway           gateways.Gateway
	configMapProvider configmap.ModelConfigMapProvider
	proxyProvider     proxy.ProxyProvider
}

func NewCleanupActor(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider) plugins.ConditionActor {
	return &CleanupActor{gateway: gateway, proxyProvider: proxyProvider}
}

func (a *CleanupActor) GetType() string {
	return "TritonCleanup"
}

func (a *CleanupActor) Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error) {
	logger.Info("Retrieving Triton cleanup condition")

	// Check if infrastructure still exists
	_, err := a.gateway.GetInfrastructureStatus(ctx, logger, gateways.GetInfrastructureStatusRequest{
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

func (a *CleanupActor) Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error {
	logger.Info("Running Triton infrastructure cleanup with ConfigMap and HTTPRoute cleanup")

	// STEP 1: Delete ConfigMaps first (following UCS cleanup pattern)
	logger.Info("Cleaning up ConfigMaps for inference server", zap.String("inferenceServer", resource.Name))

	// Clean up model-config ConfigMap
	modelConfigMapName := fmt.Sprintf("%s-model-config", resource.Name)
	if err := a.configMapProvider.DeleteModelConfigMap(ctx, configmap.DeleteModelConfigMapRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	}); err != nil {
		logger.Error("Failed to delete model ConfigMap", zap.String("configMap", modelConfigMapName), zap.Error(err))
		// Don't fail the whole cleanup for ConfigMap errors, but log them
	} else {
		logger.Info("Successfully deleted model ConfigMap", zap.String("configMap", modelConfigMapName))
	}

	// Note: No longer using deployment-registry ConfigMap, using only shared model-config ConfigMap

	// STEP 2: Delete HTTPRoute for the inference server
	logger.Info("Cleaning up HTTPRoute for inference server", zap.String("inferenceServer", resource.Name))
	httpRouteName := fmt.Sprintf("%s-httproute", resource.Name)
	if err := a.proxyProvider.DeleteInferenceServerRoute(ctx, logger, proxy.DeleteInferenceServerRouteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	}); err != nil {
		logger.Error("Failed to delete HTTPRoute", zap.String("httpRoute", httpRouteName), zap.Error(err))
		// Don't fail the whole cleanup for HTTPRoute errors, but log them
	} else {
		logger.Info("Successfully deleted HTTPRoute", zap.String("httpRoute", httpRouteName))
	}

	// STEP 3: Delete infrastructure (Kubernetes resources like Deployment, Service, etc.)
	logger.Info("Cleaning up Kubernetes infrastructure", zap.String("inferenceServer", resource.Name))
	err := a.gateway.DeleteInfrastructure(ctx, logger, gateways.DeleteInfrastructureRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "InfrastructureCleanupFailed"
		condition.Message = fmt.Sprintf("Failed to cleanup infrastructure: %v", err)
		return err
	}

	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "CleanupInitiated"
	condition.Message = "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully"
	logger.Info("Triton infrastructure cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return nil
}
