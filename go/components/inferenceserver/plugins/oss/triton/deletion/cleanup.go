package deletion

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &CleanupActor{}

// CleanupActor removes all Kubernetes resources associated with a Triton inference server.
type CleanupActor struct {
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	proxyProvider          proxy.ProxyProvider
	logger                 *zap.Logger
}

// NewCleanupActor creates a condition actor for infrastructure cleanup during deletion.
func NewCleanupActor(gateway gateways.Gateway, modelConfigMapProvider configmap.ModelConfigMapProvider, proxyProvider proxy.ProxyProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &CleanupActor{
		gateway:                gateway,
		modelConfigMapProvider: modelConfigMapProvider,
		proxyProvider:          proxyProvider,
		logger:                 logger,
	}
}

// GetType returns the condition type identifier for cleanup.
func (a *CleanupActor) GetType() string {
	return common.TritonCleanupConditionType
}

// Retrieve checks if all infrastructure has been successfully deleted.
func (a *CleanupActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton cleanup condition")

	// Check if infrastructure still exists
	_, err := a.gateway.GetInfrastructureStatus(ctx, a.logger, gateways.GetInfrastructureStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		BackendType:     resource.Spec.BackendType,
	})

	if err == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "CleanupInProgress",
			Message: "Infrastructure cleanup in progress",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: "Infrastructure cleanup completed",
	}, nil
}

// Run deletes the deployment, service, ConfigMaps, and HTTPRoute for the inference server.
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
		a.logger.Error("Failed to delete model ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_configmap"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("configMap", modelConfigMapName))
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
		a.logger.Error("Failed to delete HTTPRoute",
			zap.Error(err),
			zap.String("operation", "delete_httproute"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name),
			zap.String("httpRoute", httpRouteName))
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
		a.logger.Error("Failed to delete infrastructure",
			zap.Error(err),
			zap.String("operation", "delete_infrastructure"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InfrastructureCleanupFailed",
			Message: fmt.Sprintf("Failed to cleanup infrastructure: %v", err),
		}, fmt.Errorf("delete infrastructure for inference server %s/%s: %w", resource.Namespace, resource.Name, err)
	}

	a.logger.Info("Triton infrastructure cleanup completed successfully", zap.String("inferenceServer", resource.Name))
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupInitiated",
		Message: "Infrastructure, model ConfigMap, and HTTPRoute cleanup initiated successfully",
	}, nil
}
