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

// TrafficRoutingActor manages HTTPRoute configuration to route deployment traffic to models.
type TrafficRoutingActor struct {
	ProxyProvider proxy.ProxyProvider
	Logger        *zap.Logger
}

// GetType returns the condition type identifier for traffic routing.
func (a *TrafficRoutingActor) GetType() string {
	return common.ActorTypeTrafficRouting
}

// GetLogger returns the logger instance for this actor.
func (a *TrafficRoutingActor) GetLogger() *zap.Logger {
	return a.Logger
}

// Retrieve checks if the Gateway API HTTPRoute is correctly configured for the deployment.
func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Retrieving traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	ok, err := a.ProxyProvider.CheckDeploymentRouteStatus(ctx, a.Logger, proxy.CheckDeploymentRouteStatusRequest{
		DeploymentName:  deployment.Name,
		Namespace:       deployment.Namespace,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		ModelName:       deployment.Spec.DesiredRevision.Name,
	})
	if err != nil {
		a.Logger.Error("failed to check deployment route status",
			zap.Error(err),
			zap.String("operation", "check_deployment_route_status"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "CheckDeploymentRouteStatusFailed",
			Message: fmt.Sprintf("Failed to check deployment route status: %v", err),
		}, nil
	}

	if !ok {
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "DeploymentRouteNotConfigured", Message: "Deployment route is not configured"}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "TrafficRoutingConfigured",
		Message: fmt.Sprintf("HTTPRoute %s successfully configured for deployment", deployment.Name),
	}, nil
}

// Run creates or updates the HTTPRoute to enable traffic routing to the deployed model.
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

	err := a.ProxyProvider.EnsureDeploymentRoute(ctx, a.Logger, proxy.EnsureDeploymentRouteRequest{
		DeploymentName:  deployment.Name,
		Namespace:       deployment.Namespace,
		ModelName:       deployment.Spec.DesiredRevision.Name,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
	})
	if err != nil {
		a.Logger.Error("failed to add deployment route",
			zap.Error(err),
			zap.String("operation", "ensure_deployment_route"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
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
