package common

import (
	"context"
	"fmt"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Condition reasons reported by TrafficRoutingActor.
const (
	ReasonTrafficRouteNotReady   = "TrafficRouteNotReady"
	ReasonModelNotPromoted       = "ModelNotPromoted"
	ReasonModelEntryMissing      = "ModelEntryMissing"
	ReasonWaitingForReplicas     = "WaitingForReplicas"
	ReasonModelConfigReadFailed  = "ModelConfigReadFailed"
	ReasonPromoteModelFailed     = "PromoteModelFailed"
	ReasonTrafficRouteUpsertFail = "TrafficRouteUpsertFailed"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &TrafficRoutingActor{}

// TrafficRoutingActor switches a cluster's traffic to the new model. It first promotes the
// model config entry to the serving phase, so from now on a replica is only ready when it
// has the model, then confirms every replica still reports the model ready, and only then
// points the deployment's rule on the per-cluster traffic HTTPRoute at the model. Because
// the rule is shared by every replica behind the inference Service, the flip must wait
// for all of them rather than the one a Service round-robin would have hit. One instance
// is created per target cluster at actor-chain construction time.
type TrafficRoutingActor struct {
	deps   ClusterActorDeps
	target *v2pb.ClusterTarget
}

// NewTrafficRoutingActor creates a TrafficRoutingActor for the given cluster.
func NewTrafficRoutingActor(deps ClusterActorDeps, target *v2pb.ClusterTarget) *TrafficRoutingActor {
	return &TrafficRoutingActor{deps: deps, target: target}
}

// GetType returns the condition type identifier, including the cluster ID so each
// cluster gets its own condition entry in status.conditions.
func (a *TrafficRoutingActor) GetType() string {
	return osscommon.ActorTypeTrafficRouting + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the deployment's rule on the cluster's traffic HTTPRoute routes to
// the desired model and the model's config entry is in the serving phase. Returns FALSE on
// a desiredRevision change so Run reapplies the rule body.
func (a *TrafficRoutingActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	dynamicClient, err := a.deps.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()
	clusterID := a.target.GetClusterId()
	rule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(isName, deployment.Name),
		RewritePath: routenames.TrafficRewritePath(modelName),
	}
	ok, err := a.deps.RouteManager.RuleExists(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, rule)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteStatusCheckFailed", err.Error()), nil
	}
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, ReasonTrafficRouteNotReady, fmt.Sprintf("traffic route for deployment %s is not configured for model %s in cluster %s", deployment.Name, modelName, clusterID)), nil
	}

	kubeClient, err := a.deps.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, osscommon.ReasonClientUnavailable, err.Error()), nil
	}
	entries, err := a.deps.ModelConfigProvider.GetModelsFromConfig(ctx, a.deps.Logger, kubeClient, isName, deployment.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelConfigReadFailed, err.Error()), nil
	}
	entry, ok := modelconfig.FindEntry(entries, deployment.Name, modelName)
	if !ok || entry.EffectivePhase() != modelconfig.ModelPhaseServing {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelNotPromoted, fmt.Sprintf("model %s is not in the serving phase in cluster %s", modelName, clusterID)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run promotes the model to the serving phase, re-checks that every replica serves it, and
// then adds or updates the deployment's rule on the cluster's traffic HTTPRoute. If a
// replica is not serving the model (for example it restarted since the load completed), the
// rule is left on the previous model and the actor waits.
func (a *TrafficRoutingActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if osscommon.IsTerminalFailure(condition, ReasonModelLoadFailed) {
		return condition, nil
	}

	kubeClient, err := a.deps.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, osscommon.ReasonClientUnavailable, err.Error()), nil
	}
	dynamicClient, err := a.deps.ClientFactory.GetDynamicClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DynamicClientUnavailable", err.Error()), nil
	}

	isName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()
	clusterID := a.target.GetClusterId()

	entries, err := a.deps.ModelConfigProvider.GetModelsFromConfig(ctx, a.deps.Logger, kubeClient, isName, deployment.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelConfigReadFailed, err.Error()), nil
	}
	entry, ok := modelconfig.FindEntry(entries, deployment.Name, modelName)
	if !ok {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelEntryMissing, fmt.Sprintf("model %s has no entry in the model config of cluster %s; it must be loaded before traffic is routed to it", modelName, clusterID)), nil
	}
	if entry.EffectivePhase() != modelconfig.ModelPhaseServing {
		entry.Phase = modelconfig.ModelPhaseServing
		entry.CanaryPod = ""
		if err := a.deps.ModelConfigProvider.AddModelToConfig(ctx, a.deps.Logger, kubeClient, isName, deployment.Namespace, entry); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, ReasonPromoteModelFailed, err.Error()), nil
		}
	}

	status, failure := a.deps.modelStatus(ctx, a.target, deployment, modelName)
	if failure != nil {
		return conditionsutil.GenerateFalseCondition(condition, failure.Reason, failure.Message), nil
	}
	if failed := status.Failed(); len(failed) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelLoadFailed,
			fmt.Sprintf("model %s failed to load in cluster %s on %s", modelName, clusterID, describeFailedReplicas(failed))), nil
	}
	if !status.Ready() {
		return conditionsutil.GenerateUnknownCondition(condition, ReasonWaitingForReplicas,
			fmt.Sprintf("not routing traffic to model %s in cluster %s until every replica serves it: %s", modelName, clusterID, status.Summary())), nil
	}

	rule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(isName, deployment.Name),
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.TrafficRewritePath(modelName),
		RewriteType: routing.RewritePrefix,
		BackendName: isName + "-inference-service",
	}
	if err := a.deps.RouteManager.AddRules(ctx, dynamicClient, routenames.TrafficRouteName(isName), deployment.Namespace, rule); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonTrafficRouteUpsertFail, err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
