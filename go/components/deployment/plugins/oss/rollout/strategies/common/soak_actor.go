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

// Condition reasons reported by SoakActor.
const (
	ReasonSoakNotStarted = "SoakNotStarted"
	ReasonSoaking        = "Soaking"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &SoakActor{}

// SoakActor holds the rollout after a cluster has switched traffic to the new model, for
// the strategy's rollout period, before the next cluster starts. The controller keeps
// evaluating the health and metric gates on every reconcile during the hold, so a model
// that misbehaves under real traffic is rolled back while it is still confined to this
// cluster. One instance is created per cluster.
type SoakActor struct {
	deps   ClusterActorDeps
	target *v2pb.ClusterTarget
}

// NewSoakActor creates a SoakActor for the given cluster.
func NewSoakActor(deps ClusterActorDeps, target *v2pb.ClusterTarget) *SoakActor {
	return &SoakActor{deps: deps, target: target}
}

// GetType returns the condition type identifier, including the cluster ID so each cluster
// gets its own condition entry in status.conditions.
func (a *SoakActor) GetType() string {
	return osscommon.ActorTypeSoak + "-" + a.target.GetClusterId()
}

// Retrieve reports TRUE once the soak period has elapsed since Run started it.
func (a *SoakActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	period := a.deps.Settings.SoakPeriod
	if period <= 0 {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

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
		return conditionsutil.GenerateFalseCondition(condition, ReasonSoakNotStarted,
			fmt.Sprintf("soak of model %s not started in cluster %s", modelName, clusterID)), nil
	}

	elapsed := progress.Elapsed(a.deps.now())
	if elapsed >= period {
		progress.Done = true
		progress.CompletedAt = a.deps.now().Unix()
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
		return conditionsutil.GenerateTrueCondition(condition), nil
	}
	return conditionsutil.GenerateFalseCondition(condition, ReasonSoaking,
		fmt.Sprintf("cluster %s soaking model %s: %s of %s elapsed", clusterID, modelName, elapsed.Truncate(1e9), period)), nil
}

// Run starts the soak clock if it is not running and returns UNKNOWN so the engine keeps
// polling via Retrieve.
func (a *SoakActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	progress, err := osscommon.ReadRolloutProgress(condition)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "MetadataReadFailed", err.Error()), nil
	}
	if !progress.Started() {
		progress.StartedAt = a.deps.now().Unix()
		if err := osscommon.WriteRolloutProgress(condition, progress); err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "MetadataWriteFailed", err.Error()), nil
		}
	}

	remaining := a.deps.Settings.SoakPeriod - progress.Elapsed(a.deps.now())
	if remaining < 0 {
		remaining = 0
	}
	return conditionsutil.GenerateUnknownCondition(condition, ReasonSoaking,
		fmt.Sprintf("cluster %s soaking model %s: %s remaining", a.target.GetClusterId(), deployment.Spec.GetDesiredRevision().GetName(), remaining.Truncate(1e9))), nil
}
