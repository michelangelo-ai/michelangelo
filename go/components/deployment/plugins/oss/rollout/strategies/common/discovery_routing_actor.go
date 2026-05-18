package common

import (
	"context"

	"k8s.io/client-go/dynamic"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &DiscoveryRoutingActor{}

// DiscoveryRoutingActor adds the deployment's rule to the control-plane discovery
// HTTPRoute that the InferenceServer controller owns. The rule matches
// /{inferenceServerName}/{deploymentName} and routes it onward to the deployment's
// model. A single instance is created per Deployment.
type DiscoveryRoutingActor struct {
	controlPlaneDynamicClient dynamic.Interface
	routeManager              routing.Manager
}

// NewDiscoveryRoutingActor creates a DiscoveryRoutingActor.
func NewDiscoveryRoutingActor(
	controlPlaneDynamicClient dynamic.Interface,
	routeManager routing.Manager,
) *DiscoveryRoutingActor {
	return &DiscoveryRoutingActor{
		controlPlaneDynamicClient: controlPlaneDynamicClient,
		routeManager:              routeManager,
	}
}

// GetType returns the condition type identifier for the discovery routing actor.
func (a *DiscoveryRoutingActor) GetType() string {
	return osscommon.ActorTypeDiscoveryRouting
}

// Retrieve checks whether the deployment's rule is present on the discovery HTTPRoute.
func (a *DiscoveryRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	isName := deployment.Spec.GetInferenceServer().GetName()
	rule := routing.Rule{MatchPath: routenames.DiscoveryMatchPath(isName, deployment.Name)}
	ok, err := a.routeManager.RuleExists(ctx, a.controlPlaneDynamicClient, routenames.DiscoveryRouteName(isName), deployment.Namespace, rule)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteStatusCheckFailed", err.Error()), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteNotReady", "discovery route is not configured for the deployment"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run adds or updates the deployment's rule on the discovery HTTPRoute.
func (a *DiscoveryRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	isName := deployment.Spec.GetInferenceServer().GetName()
	rule := routing.Rule{
		MatchPath:   routenames.DiscoveryMatchPath(isName, deployment.Name),
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.DiscoveryRewritePath(isName, deployment.Name),
		RewriteType: routing.RewritePrefix,
		BackendName: isName + "-endpoints",
	}
	if err := a.routeManager.AddRules(ctx, a.controlPlaneDynamicClient, routenames.DiscoveryRouteName(isName), deployment.Namespace, rule); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteUpsertFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
