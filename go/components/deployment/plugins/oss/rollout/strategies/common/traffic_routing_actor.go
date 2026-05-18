package common

import (
	"context"
	"fmt"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &TrafficRoutingActor{}

// TrafficRoutingActor adds the deployment's rule to the per-cluster traffic
// HTTPRoute that the InferenceServer controller owns. The rule routes
// /{inferenceServerName}/{deploymentName} to the deployment's model on the
// local inference Service. One instance is created per target cluster at
// actor-chain construction time.
type TrafficRoutingActor struct {
	clientFactory clientfactory.ClientFactory
	routeManager  routing.Manager
	target        *v2pb.ClusterTarget
}

// NewTrafficRoutingActor creates a TrafficRoutingActor for the given cluster.
func NewTrafficRoutingActor(
	clientFactory clientfactory.ClientFactory,
	routeManager routing.Manager,
	target *v2pb.ClusterTarget,
) *TrafficRoutingActor {
	return &TrafficRoutingActor{
		clientFactory: clientFactory,
		routeManager:  routeManager,
		target:        target,
	}
}

// GetType returns the condition type identifier, including the cluster ID so each
// cluster gets its own condition entry in status.conditions.
func (a *TrafficRoutingActor) GetType() string {
	return osscommon.ActorTypeTrafficRouting + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the deployment's rule is present on the cluster's
// traffic HTTPRoute and routes to the deployment's currently desired model.
// Returns FALSE on a desiredRevision change so Run reapplies the rule body.
func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()
	rule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(isName, deployment.Name),
		RewritePath: routenames.TrafficRewritePath(modelName),
	}
	ok, err := a.routeManager.RuleExists(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, rule)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteStatusCheckFailed", err.Error()), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteNotReady", fmt.Sprintf("traffic route for deployment %s is not configured for model %s in cluster %s", deployment.Name, modelName, a.target.GetClusterId())), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run adds or updates the deployment's rule on the cluster's traffic HTTPRoute.
func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()
	rule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(isName, deployment.Name),
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.TrafficRewritePath(modelName),
		RewriteType: routing.RewritePrefix,
		BackendName: isName + "-inference-service",
	}
	if err := a.routeManager.AddRules(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, rule); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteUpsertFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
