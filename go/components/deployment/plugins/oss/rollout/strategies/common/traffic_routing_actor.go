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

// TrafficRoutingActor creates or updates the HTTPRoute in a single target cluster so that
// traffic is directed to the desired model revision. One instance is created per cluster
// at actor-chain construction time.
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

// Run creates or updates the cluster's HTTPRoute to route traffic to the desired model.
func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	dynamicClient, err := a.params.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	if err := a.params.RouteProvider.EnsureDeploymentRoute(ctx, a.params.Logger, dynamicClient, deployment.Name, deployment.Namespace, inferenceServerName, modelName); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "RouteEnsureFailed", err.Error()), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Retrieve checks whether the cluster's HTTPRoute is correctly configured for the desired model.
func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	dynamicClient, err := a.params.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	ok, err := a.params.RouteProvider.CheckDeploymentRouteStatus(ctx, a.params.Logger, dynamicClient, deployment.Name, deployment.Namespace, inferenceServerName, modelName)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "RouteStatusCheckFailed", err.Error()), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "RouteNotReady", fmt.Sprintf("HTTPRoute in cluster %s not pointing at model %s", a.target.GetClusterId(), modelName)), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}
