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

// ProxyConfigurationActor configures Istio proxy
type ProxyConfigurationActor struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
	logger        *zap.Logger
}

func NewProxyConfigurationActor(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ProxyConfigurationActor{
		gateway:       gateway,
		proxyProvider: proxyProvider,
		logger:        logger,
	}
}

func (a *ProxyConfigurationActor) GetType() string {
	return common.TritonProxyConfigurationConditionType
}

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

func (a *ProxyConfigurationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running Triton proxy configuration")

	err := a.proxyProvider.EnsureInferenceServerRoute(ctx, a.logger, proxy.EnsureInferenceServerRouteRequest{
		InferenceServer: resource.Name,
		Namespace:       resource.Namespace,
		ModelName:       resource.Name,
	})
	if err != nil {
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
