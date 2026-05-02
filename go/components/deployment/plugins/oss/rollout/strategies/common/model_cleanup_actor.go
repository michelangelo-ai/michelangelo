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

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ModelCleanupActor{}

// ModelCleanupActor removes the previous model revision from a single cluster's inference
// server ConfigMap after all traffic has been routed to the new revision. One instance is
// created per cluster at actor-chain construction time.
type ModelCleanupActor struct {
	params Params
	target *v2pb.ClusterTarget
}

// NewModelCleanupActor creates a ModelCleanupActor for the given cluster.
func NewModelCleanupActor(params Params, target *v2pb.ClusterTarget) *ModelCleanupActor {
	return &ModelCleanupActor{params: params, target: target}
}

// GetType returns the condition type identifier, including the cluster ID so each
// cluster gets its own condition entry in status.conditions.
func (a *ModelCleanupActor) GetType() string {
	return osscommon.ActorTypeModelCleanup + "-" + a.target.GetClusterId()
}

// noCleanupNeeded returns true when there is no prior revision to remove, or when the prior
// revision is the same as the desired revision (idempotent re-deploy of the same model).
func noCleanupNeeded(deployment *v2pb.Deployment) bool {
	currentRevision := deployment.Status.GetCurrentRevision()
	if currentRevision == nil {
		return true
	}
	return currentRevision.GetName() == deployment.Spec.GetDesiredRevision().GetName()
}

// Retrieve checks whether the previous model revision has been unloaded from Triton.
// Returns TRUE immediately if this is the first rollout (no prior revision).
func (a *ModelCleanupActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if noCleanupNeeded(deployment) {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	kubeClient, err := a.params.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	httpClient, err := a.params.ClientFactory.GetHTTPClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "HTTPClientUnavailable", err.Error()), nil
	}

	backend, err := a.params.BackendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "BackendUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	oldModel := deployment.Status.GetCurrentRevision().GetName()

	stillLoaded, err := backend.CheckModelStatus(ctx, a.params.Logger, kubeClient, httpClient, inferenceServerName, deployment.Namespace, oldModel)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ModelStatusCheckFailed", err.Error()), nil
	}
	if stillLoaded {
		return conditionsutil.GenerateFalseCondition(condition, "OldModelStillLoaded", fmt.Sprintf("model %s still loaded in cluster %s", oldModel, a.target.GetClusterId())), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run removes the previous model revision from the cluster's inference server ConfigMap,
// triggering the server to begin unloading it. Returns UNKNOWN so the engine continues
// polling via Retrieve. Returns TRUE immediately if there is no prior revision to remove.
func (a *ModelCleanupActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if noCleanupNeeded(deployment) {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	kubeClient, err := a.params.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ClientUnavailable", err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	oldModel := deployment.Status.GetCurrentRevision().GetName()

	if err := a.params.ModelConfigProvider.RemoveModelFromConfig(ctx, a.params.Logger, kubeClient, inferenceServerName, deployment.Namespace, oldModel); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "RemoveModelFromConfigFailed", err.Error()), nil
	}

	return conditionsutil.GenerateUnknownCondition(condition, "ModelUnloading", fmt.Sprintf("model %s unloading from cluster %s", oldModel, a.target.GetClusterId())), nil
}
