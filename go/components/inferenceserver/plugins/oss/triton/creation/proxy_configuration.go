package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ProxyConfigurationActor{}

// ProxyConfigurationActor configures HTTP routing for inference server traffic.
type ProxyConfigurationActor struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
	logger        *zap.Logger
}

// NewProxyConfigurationActor creates a condition actor for configuring Gateway API HTTPRoutes.
func NewProxyConfigurationActor(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ProxyConfigurationActor{
		gateway:       gateway,
		proxyProvider: proxyProvider,
		logger:        logger,
	}
}

// GetType returns the condition type identifier for proxy configuration.
func (a *ProxyConfigurationActor) GetType() string {
	return common.TritonProxyConfigurationConditionType
}

// Retrieve checks if the Gateway API HTTPRoute is configured for the inference server.
func (a *ProxyConfigurationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton proxy configuration condition")

	proxyStatus, err := a.proxyProvider.GetProxyStatus(ctx, a.logger, proxy.GetProxyStatusRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
	})

	if err == nil && proxyStatus.Status.Configured {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ProxyConfigured",
			Message: "Proxy is configured and ready",
		}, nil
	} else if err != nil {
		a.logger.Error("Failed to check proxy status",
			zap.Error(err),
			zap.String("operation", "get_proxy_status"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ProxyNotConfigured",
			Message: fmt.Sprintf("Failed to check proxy status: %v", err),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ProxyNotConfigured",
		Message: "Proxy is not configured",
	}, nil
}

// Run creates or updates the HTTPRoute to enable external traffic routing.
func (a *ProxyConfigurationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton proxy configuration")

	err := a.proxyProvider.EnsureInferenceServerRoute(ctx, a.logger, proxy.EnsureInferenceServerRouteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		ModelName:       resource.Name,
	})
	if err != nil {
		a.logger.Error("Failed to configure proxy",
			zap.Error(err),
			zap.String("operation", "ensure_inference_server_route"),
			zap.String("namespace", resource.Namespace),
			zap.String("inferenceServer", resource.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ProxyConfigurationFailed",
			Message: fmt.Sprintf("Failed to configure proxy: %v", err),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ProxyConfigured",
		Message: "Proxy configured successfully",
	}, nil
}
