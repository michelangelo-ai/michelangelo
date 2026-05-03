package common

import (
	"context"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelDiscoveryActor{}

// ModelDiscoveryActor writes the control-plane HTTPRoute that exposes the deployment's model
// across all clusters hosting the inference server. The route forwards traffic to the inference
// server's discovery Service ({inferenceServerName}-endpoints), whose EndpointSlices fan out to
// each hosting cluster's gateway. A single instance is created per Deployment.
type ModelDiscoveryActor struct {
	params Params
}

// NewModelDiscoveryActor creates a ModelDiscoveryActor.
func NewModelDiscoveryActor(params Params) *ModelDiscoveryActor {
	return &ModelDiscoveryActor{params: params}
}

// GetType returns the condition type identifier for the model discovery actor.
func (a *ModelDiscoveryActor) GetType() string {
	return osscommon.ActorTypeModelDiscovery
}

// Retrieve checks whether the discovery HTTPRoute is configured for the desired model.
func (a *ModelDiscoveryActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	ok, err := a.params.ModelDiscoveryProvider.CheckDiscoveryRouteStatus(ctx, deployment.Name, deployment.Namespace, inferenceServerName, modelName)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteStatusCheckFailed", err.Error()), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteNotReady", "discovery HTTPRoute is not configured for the desired model"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run creates or updates the discovery HTTPRoute on the control-plane gateway.
func (a *ModelDiscoveryActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	if err := a.params.ModelDiscoveryProvider.EnsureDiscoveryRoute(ctx, deployment.Name, deployment.Namespace, inferenceServerName, modelName); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteEnsureFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
