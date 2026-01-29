package common

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &TrafficRoutingActor{}

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

	ok, err := a.ProxyProvider.CheckDeploymentRouteStatus(ctx, a.Logger,
		deployment.Name, deployment.Namespace, deployment.Spec.GetInferenceServer().Name, deployment.Spec.DesiredRevision.Name)
	if err != nil {
		a.Logger.Error("failed to check deployment route status",
			zap.Error(err),
			zap.String("operation", "check_deployment_route_status"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return conditionsutil.GenerateFalseCondition(condition, "CheckDeploymentRouteStatusFailed", fmt.Sprintf("Failed to check deployment route status: %v", err)), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "DeploymentRouteNotConfigured", "Deployment route is not configured"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run creates or updates the HTTPRoute to enable traffic routing to the deployed model.
func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running traffic routing configuration for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.GetInferenceServer() == nil {
		return conditionsutil.GenerateFalseCondition(condition, "MissingInferenceServer", fmt.Sprintf("inference server not specified for deployment %s", deployment.Name)), nil
	}

	err := a.ProxyProvider.EnsureDeploymentRoute(ctx, a.Logger, deployment.Name, deployment.Namespace, deployment.Spec.GetInferenceServer().Name, deployment.Spec.DesiredRevision.Name)
	if err != nil {
		a.Logger.Error("failed to add deployment route",
			zap.Error(err),
			zap.String("operation", "ensure_deployment_route"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return conditionsutil.GenerateFalseCondition(condition, "AddDeploymentRouteFailed", fmt.Sprintf("Failed to add deployment route: %v", err)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
