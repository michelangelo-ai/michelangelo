package common

import (
	"context"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &DiscoveryRoutingActor{}

// DiscoveryRoutingActor adds the deployment's rule to the control-plane discovery
// HTTPRoute that the InferenceServer controller owns. The rule matches
// /{inferenceServerName}/{deploymentName} and routes it onward to the deployment's
// model. A single instance is created per Deployment.
type DiscoveryRoutingActor struct {
	params Params
}

// NewDiscoveryRoutingActor creates a DiscoveryRoutingActor.
func NewDiscoveryRoutingActor(params Params) *DiscoveryRoutingActor {
	return &DiscoveryRoutingActor{params: params}
}

// GetType returns the condition type identifier for the discovery routing actor.
func (a *DiscoveryRoutingActor) GetType() string {
	return osscommon.ActorTypeDiscoveryRouting
}

// Retrieve checks whether the deployment's rule is present on the discovery HTTPRoute.
func (a *DiscoveryRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()

	ok, err := a.params.RouteProvider.DeploymentDiscoveryRouteExists(ctx, a.params.ControlPlaneDynamicClient, inferenceServerName, deployment.Namespace, deployment.Name)
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
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	if err := a.params.RouteProvider.UpsertDiscoveryRule(ctx, a.params.ControlPlaneDynamicClient, inferenceServerName, deployment.Namespace, deployment.Name, modelName); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteUpsertFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
