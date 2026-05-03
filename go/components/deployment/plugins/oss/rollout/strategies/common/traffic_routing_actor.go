package common

import (
	"context"
	"fmt"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
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
	params Params
	target *v2pb.ClusterTarget
}

// NewTrafficRoutingActor creates a TrafficRoutingActor for the given cluster.
func NewTrafficRoutingActor(params Params, target *v2pb.ClusterTarget) *TrafficRoutingActor {
	return &TrafficRoutingActor{params: params, target: target}
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
	dynamicClient, err := a.params.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	ok, err := a.params.RouteProvider.DeploymentTrafficRouteExists(ctx, dynamicClient, a.target.GetClusterId(), inferenceServerName, deployment.Namespace, deployment.Name, modelName)
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
	dynamicClient, err := a.params.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	if err := a.params.RouteProvider.UpsertTrafficRule(ctx, dynamicClient, a.target.GetClusterId(), inferenceServerName, deployment.Namespace, deployment.Name, modelName); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteUpsertFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
