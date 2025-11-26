package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor cleans up Triton infrastructure
type CleanupActor struct {
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	proxyProvider          proxy.ProxyProvider
	logger                 *zap.Logger
}

func NewCleanupActor(gateway gateways.Gateway, modelConfigMapProvider configmap.ModelConfigMapProvider, proxyProvider proxy.ProxyProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		gateway:                gateway,
		modelConfigMapProvider: modelConfigMapProvider,
		proxyProvider:          proxyProvider,
		logger:                 logger,
	}
}

func (a *CleanupActor) GetType() string {
	return "TritonCleanup"
}

func (a *CleanupActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton cleanup condition")

	// Check if infrastructure still exists
	_, err := a.gateway.GetInfrastructureStatus(ctx, a.logger, gateways.GetInfrastructureStatusRequest{
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

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  status,
		Reason:  reason,
		Message: message,
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton infrastructure cleanup with ConfigMap and HTTPRoute cleanup")

	// Delete ConfigMaps first
	a.logger.Info("Cleaning up ConfigMaps for inference server", zap.String("inferenceServer", resource.Name))

	// Clean up model-config ConfigMap
	modelConfigMapName := fmt.Sprintf("%s-model-config", resource.Name)
	if err := a.modelConfigMapProvider.DeleteModelConfigMap(ctx, configmap.DeleteModelConfigMapRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	}); err != nil {
		a.logger.Error("Failed to delete model ConfigMap", zap.String("configMap", modelConfigMapName), zap.Error(err))
		// Don't fail the whole cleanup for ConfigMap errors, but log them
	} else {
		a.logger.Info("Successfully deleted model ConfigMap", zap.String("configMap", modelConfigMapName))
	}

	// Delete HTTPRoute for the inference server
	a.logger.Info("Cleaning up HTTPRoute for inference server", zap.String("inferenceServer", resource.Name))
	httpRouteName := fmt.Sprintf("%s-httproute", resource.Name)
	if err := a.proxyProvider.DeleteInferenceServerRoute(ctx, a.logger, proxy.DeleteInferenceServerRouteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	}); err != nil {
		a.logger.Error("Failed to delete HTTPRoute", zap.String("httpRoute", httpRouteName), zap.Error(err))
		// Don't fail the whole cleanup for HTTPRoute errors, but log them
	} else {
		a.logger.Info("Successfully deleted HTTPRoute", zap.String("httpRoute", httpRouteName))
	}

	// Delete infrastructure (Kubernetes resources like Deployment, Service, etc.)
	a.logger.Info("Cleaning up Kubernetes infrastructure", zap.String("inferenceServer", resource.Name))
	err := a.gateway.DeleteInfrastructure(ctx, a.logger, gateways.DeleteInfrastructureRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})
	if err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "InfrastructureCleanupFailed"
		condition.Message = fmt.Sprintf("Failed to cleanup infrastructure: %v", err)
		return nil, err
	}

	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "CleanupInitiated"
	condition.Message = "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully"
	a.logger.Info("Triton infrastructure cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return condition, nil
}
