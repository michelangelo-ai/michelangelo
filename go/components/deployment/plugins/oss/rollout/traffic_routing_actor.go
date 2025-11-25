package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TrafficRoutingActor handles HTTPRoute management for deployment traffic routing
type TrafficRoutingActor struct {
	ProxyProvider proxy.ProxyProvider
	Logger        *zap.Logger
}

func (a *TrafficRoutingActor) GetType() string {
	return common.ActorTypeTrafficRouting
}

func (a *TrafficRoutingActor) GetLogger() *zap.Logger {
	return a.Logger
}

func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Retrieving traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	if ok, err := a.ProxyProvider.CheckDeploymentRouteStatus(ctx, a.Logger, proxy.CheckDeploymentRouteStatusRequest{
		DeploymentName:  deployment.Name,
		Namespace:       deployment.Namespace,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		ModelName:       deployment.Spec.DesiredRevision.Name,
	}); err != nil {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "CheckDeploymentRouteStatusFailed", Message: fmt.Sprintf("Failed to check deployment route status: %v", err)}, nil
	} else if !ok {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "DeploymentRouteNotConfigured", Message: "Deployment route is not configured"}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "TrafficRoutingConfigured",
		Message: fmt.Sprintf("HTTPRoute %s successfully configured for deployment", deployment.Name),
	}, nil
}

func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "MissingInferenceServer",
			Message: fmt.Sprintf("inference server not specified for deployment %s", deployment.Name),
		}, nil
	}

	if err := a.ProxyProvider.EnsureDeploymentRoute(ctx, a.Logger, proxy.EnsureDeploymentRouteRequest{
		DeploymentName:  deployment.Name,
		Namespace:       deployment.Namespace,
		ModelName:       deployment.Spec.DesiredRevision.Name,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
	}); err != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "AddDeploymentRouteFailed",
			Message: fmt.Sprintf("Failed to add deployment route: %v", err),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "TrafficRoutingConfigured",
		Message: fmt.Sprintf("HTTPRoute for deployment %s successfully configured", deployment.Name),
	}, nil
}
