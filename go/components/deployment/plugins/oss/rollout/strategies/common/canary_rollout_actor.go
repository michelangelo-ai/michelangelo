package common

import (
	"context"
	"errors"
	"fmt"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	plugincommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Condition reasons reported by CanaryRolloutActor.
const (
	ReasonCanaryNotStarted     = "CanaryNotStarted"
	ReasonCanaryLoading        = "CanaryLoading"
	ReasonCanaryReplicaMissing = "CanaryReplicaMissing"
	ReasonCanaryLoadFailed     = "CanaryLoadFailed"
	ReasonCanaryLoadTimeout    = "CanaryLoadTimeout"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &CanaryRolloutActor{}

// CanaryRolloutActor loads the new model on a single replica of a cluster's inference
// server and waits for that replica to report it ready. A model that cannot load, or that
// takes the replica down while loading, is contained to that one replica instead of every
// replica in the cluster. No traffic is routed to the canary: the model config entry is
// written in the canary phase, which the readiness probe ignores and the sync daemon loads
// on the named pod only. One instance is created per cluster.
type CanaryRolloutActor struct {
	deps   ClusterActorDeps
	target *v2pb.ClusterTarget
}

// NewCanaryRolloutActor creates a CanaryRolloutActor for the given cluster.
func NewCanaryRolloutActor(deps ClusterActorDeps, target *v2pb.ClusterTarget) *CanaryRolloutActor {
	return &CanaryRolloutActor{deps: deps, target: target}
}

// GetType returns the condition type identifier, including the cluster ID so each cluster
// gets its own condition entry in status.conditions.
func (a *CanaryRolloutActor) GetType() string {
	return osscommon.ActorTypeCanaryRollout + "-" + a.target.GetClusterId()
}

// Retrieve checks whether the canary replica has the model loaded. A failed load or an
// exhausted load budget is reported as a terminal FALSE so the rollout fails fast instead
// of polling forever.
func (a *CanaryRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
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
		return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryNotStarted,
			fmt.Sprintf("canary of model %s not started in cluster %s", modelName, clusterID)), nil
	}

	status, failure := a.deps.modelStatus(ctx, a.target, deployment, modelName)
	if failure != nil {
		return conditionsutil.GenerateFalseCondition(condition, failure.Reason, failure.Message), nil
	}

	timedOut := progress.TimedOut(a.deps.now(), a.deps.Settings.ModelLoadTimeout)
	replica, ok := status.Replica(progress.Replica)
	if !ok {
		if timedOut {
			return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryLoadTimeout,
				fmt.Sprintf("canary replica %s for model %s left cluster %s and no replacement was ready within %s", progress.Replica, modelName, clusterID, a.deps.Settings.ModelLoadTimeout)), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryReplicaMissing,
			fmt.Sprintf("canary replica %q for model %s is not running in cluster %s", progress.Replica, modelName, clusterID)), nil
	}

	switch replica.State {
	case backends.ModelLoadStateFailed:
		return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryLoadFailed,
			fmt.Sprintf("model %s failed to load on canary replica %s in cluster %s: %s", modelName, replica.Replica, clusterID, replica.Reason)), nil
	case backends.ModelLoadStateReady:
		progress.Done = true
		progress.CompletedAt = a.deps.now().Unix()
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	default:
		if timedOut {
			return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryLoadTimeout,
				fmt.Sprintf("model %s not loaded on canary replica %s in cluster %s within %s: %s", modelName, replica.Replica, clusterID, a.deps.Settings.ModelLoadTimeout, replica.Reason)), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, ReasonCanaryLoading,
			fmt.Sprintf("model %s loading on canary replica %s in cluster %s: %s", modelName, replica.Replica, clusterID, replica.Reason)), nil
	}
}

// Run picks the canary replica and registers the model in the cluster's model config in
// the canary phase, so only that replica loads it. Returns UNKNOWN so the engine keeps
// polling via Retrieve.
func (a *CanaryRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if osscommon.IsTerminalFailure(condition, ReasonCanaryLoadFailed, ReasonCanaryLoadTimeout) {
		return condition, nil
	}

	progress, err := osscommon.ReadRolloutProgress(condition)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataReadFailed", err.Error()), nil
	}

	modelName := deployment.Spec.GetDesiredRevision().GetName()
	inferenceServerName := deployment.Spec.GetInferenceServer().GetName()
	clusterID := a.target.GetClusterId()

	kubeClient, err := a.deps.ClientFactory.GetClient(ctx, a.target)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, osscommon.ReasonClientUnavailable, err.Error()), nil
	}

	storagePath, err := plugincommon.ResolveDeploymentModelStoragePath(ctx, a.deps.APIHandler, deployment)
	if err != nil {
		var resolutionErr *plugincommon.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			return conditionsutil.GenerateFalseCondition(condition, resolutionErr.Reason, resolutionErr.Message), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, "ModelResolutionFailed", err.Error()), nil
	}

	status, failure := a.deps.modelStatus(ctx, a.target, deployment, modelName)
	if failure != nil {
		return conditionsutil.GenerateFalseCondition(condition, failure.Reason, failure.Message), nil
	}

	if !progress.Started() {
		progress.StartedAt = a.deps.now().Unix()
	}
	canary := pickCanaryReplica(status, progress.Replica)
	if canary == "" {
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
		return conditionsutil.GenerateUnknownCondition(condition, ReasonCanaryLoading,
			fmt.Sprintf("waiting for a running replica in cluster %s to canary model %s: %s", clusterID, modelName, status.Summary())), nil
	}
	progress.Replica = canary

	if err := a.deps.ModelConfigProvider.AddModelToConfig(ctx, a.deps.Logger, kubeClient, inferenceServerName, deployment.Namespace, modelconfig.ModelConfigEntry{
		Name:           modelName,
		StoragePath:    storagePath,
		DeploymentName: deployment.GetName(),
		Phase:          modelconfig.ModelPhaseCanary,
		CanaryPod:      canary,
	}); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "AddModelToConfigFailed", err.Error()), nil
	}

	if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
	}
	return conditionsutil.GenerateUnknownCondition(condition, ReasonCanaryLoading,
		fmt.Sprintf("model %s loading on canary replica %s in cluster %s", modelName, canary, clusterID)), nil
}

// pickCanaryReplica keeps the previously chosen replica while it is still running and
// otherwise picks the first running replica by name, so every reconcile agrees on the
// same pod. Returns "" when no replica is running.
func pickCanaryReplica(status *backends.ModelStatus, previous string) string {
	if previous != "" {
		if replica, ok := status.Replica(previous); ok && replica.Running {
			return previous
		}
	}
	for _, replica := range status.Replicas {
		if replica.Running {
			return replica.Replica
		}
	}
	return ""
}
