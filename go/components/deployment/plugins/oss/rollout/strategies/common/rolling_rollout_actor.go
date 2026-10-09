package common

import (
	"context"
	"errors"
	"fmt"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	plugincommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Condition reasons reported by RollingRolloutActor.
const (
	ReasonModelLoadNotStarted = "ModelLoadNotStarted"
	ReasonModelLoading        = "ModelLoading"
	ReasonModelNotReady       = "ModelNotReady"
	ReasonModelLoadFailed     = "ModelLoadFailed"
	ReasonModelLoadTimeout    = "ModelLoadTimeout"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads a model onto every replica of a single target cluster's
// inference server and waits until all of them report it ready. The model config entry is
// written in the staged phase: every replica loads the model, but readiness does not
// depend on it yet and no traffic is routed to it. One instance is created per cluster at
// actor-chain construction time.
type RollingRolloutActor struct {
	deps   ClusterActorDeps
	target *v2pb.ClusterTarget
}

// NewRollingRolloutActor creates a RollingRolloutActor for the given cluster.
func NewRollingRolloutActor(deps ClusterActorDeps, target *v2pb.ClusterTarget) *RollingRolloutActor {
	return &RollingRolloutActor{deps: deps, target: target}
}

// GetType returns the condition type identifier, including the cluster ID so each
// cluster gets its own condition entry in status.conditions.
func (a *RollingRolloutActor) GetType() string {
	return osscommon.ActorTypeRollingRollout + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the model is loaded and ready on every replica in the cluster.
// Once it is, that result is recorded on the condition so later calls short-circuit
// without another probe. A failed load on any replica, or an exhausted load budget, is a
// terminal FALSE so the rollout fails instead of waiting forever.
func (a *RollingRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	progress, err := osscommon.ReadRolloutProgress(condition)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataReadFailed", err.Error()), nil
	}
	if progress.Done {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	clusterID := a.target.GetClusterId()
	if !progress.Started() {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelLoadNotStarted,
			fmt.Sprintf("load of model %s not started in cluster %s", modelName, clusterID)), nil
	}

	status, failure := a.deps.modelStatus(ctx, a.target, deployment, modelName)
	if failure != nil {
		return conditionsutil.GenerateFalseCondition(condition, failure.Reason, failure.Message), nil
	}

	if failed := status.Failed(); len(failed) > 0 {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelLoadFailed,
			fmt.Sprintf("model %s failed to load in cluster %s on %s", modelName, clusterID, describeFailedReplicas(failed))), nil
	}
	if status.Ready() {
		progress.Done = true
		progress.CompletedAt = a.deps.now().Unix()
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	}
	if progress.TimedOut(a.deps.now(), a.deps.Settings.ModelLoadTimeout) {
		return conditionsutil.GenerateFalseCondition(condition, ReasonModelLoadTimeout,
			fmt.Sprintf("model %s not loaded on every replica in cluster %s within %s: %s", modelName, clusterID, a.deps.Settings.ModelLoadTimeout, status.Summary())), nil
	}
	return conditionsutil.GenerateFalseCondition(condition, ReasonModelNotReady,
		fmt.Sprintf("model %s not yet loaded in cluster %s: %s", modelName, clusterID, status.Summary())), nil
}

// Run registers the desired model in the cluster's inference server ConfigMap in the staged
// phase, triggering every replica to load it, and starts the load budget. Returns UNKNOWN
// so the engine continues polling via Retrieve.
//
// The budget starts on the first call, before any cluster access. Errors a retry may clear
// report UNKNOWN until it is spent and the terminal FALSE after; model-resolution errors fail
// at once.
func (a *RollingRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if osscommon.IsTerminalFailure(condition, ReasonModelLoadFailed, ReasonModelLoadTimeout) {
		return condition, nil
	}

	progress, err := osscommon.ReadRolloutProgress(condition)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataReadFailed", err.Error()), nil
	}

	progress = osscommon.StartClock(progress, a.deps.now())
	retry := func(reason, message string) *apipb.Condition {
		return osscommon.ReportTransient(condition, progress, a.deps.now(), a.deps.Settings.ModelLoadTimeout, ReasonModelLoadTimeout, reason, message)
	}

	kubeClient, err := a.deps.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return retry(osscommon.ReasonClientUnavailable, err.Error()), nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	modelName := deployment.Spec.GetDesiredRevision().GetName()

	storagePath, err := plugincommon.ResolveDeploymentModelStoragePath(ctx, a.deps.APIHandler, deployment)
	if err != nil {
		var resolutionErr *plugincommon.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			return conditionsutil.GenerateFalseCondition(condition, resolutionErr.Reason, resolutionErr.Message), nil
		}
		return retry("ModelResolutionFailed", err.Error()), nil
	}

	if err := a.deps.ModelConfigProvider.AddModelToConfig(ctx, a.deps.Logger, kubeClient, inferenceServerName, deployment.Namespace, modelconfig.ModelConfigEntry{
		Name:           modelName,
		StoragePath:    storagePath,
		DeploymentName: deployment.GetName(),
		Phase:          modelconfig.ModelPhaseStaged,
	}); err != nil {
		return retry("AddModelToConfigFailed", err.Error()), nil
	}

	if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
	}
	return conditionsutil.GenerateUnknownCondition(condition, ReasonModelLoading, fmt.Sprintf("model %s loading in cluster %s", modelName, a.target.GetClusterId())), nil
}
