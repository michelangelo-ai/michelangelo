package common

import (
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
)

// ShouldCleanup returns whether a deployment should be cleaned up
func ShouldCleanup(deployment types.Deployment) bool {
	// Check if DeletionSpec is set to deleted or if DeletionTimestamp is set
	if deployment.Spec.DeletionSpec != nil && deployment.Spec.DeletionSpec.Deleted {
		return true
	}
	return !deployment.ObjectMeta.DeletionTimestamp.IsZero()
}

// IsCleanupStage returns whether the deployment is in a cleanup stage
func IsCleanupStage(stage types.DeploymentStage) bool {
	return stage == types.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
}

// IsCleanupCompleteStage returns whether the deployment is in cleanup complete stage
func IsCleanupCompleteStage(stage types.DeploymentStage) bool {
	return stage == types.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
}

// RolloutInProgress returns whether a rollout is in progress
func RolloutInProgress(deployment types.Deployment) bool {
	// A rollout is in progress if we have a candidate revision different from desired
	// and we're not in a terminal or cleanup stage
	if deployment.Status.CandidateRevision == nil {
		return false
	}

	stage := deployment.Status.Stage
	return !IsTerminalStage(stage) && !IsCleanupStage(stage) && !IsCleanupCompleteStage(stage)
}

// TriggerNewRollout returns whether a new rollout should be triggered
func TriggerNewRollout(deployment types.Deployment) bool {
	// Trigger new rollout if:
	// 1. No candidate revision exists and we have a desired revision
	// 2. Desired revision differs from candidate revision
	// 3. We're not in cleanup and not already rolling out

	if deployment.Spec.DesiredRevision == nil {
		return false
	}

	if ShouldCleanup(deployment) {
		return false
	}

	if RolloutInProgress(deployment) {
		return false
	}

	// If no candidate, trigger new rollout
	if deployment.Status.CandidateRevision == nil {
		return true
	}

	// If desired differs from candidate, trigger new rollout
	return deployment.Spec.DesiredRevision.Name != deployment.Status.CandidateRevision.Name
}

// ShouldRollback returns whether a rollback should occur
func ShouldRollback(deployment types.Deployment) bool {
	// In our simplified version, we never rollback
	return false
}

// RollbackAlertsEnabled returns whether rollback alerts are enabled
func RollbackAlertsEnabled(deployment types.Deployment) bool {
	// Always return true for consistency
	return true
}

// IsRollbackStage returns whether the deployment is in a rollback stage
func IsRollbackStage(stage types.DeploymentStage) bool {
	return stage == types.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
}

// InSteadyState returns whether the deployment is in steady state
func InSteadyState(deployment types.Deployment) bool {
	// Deployment is in steady state if it's in a terminal stage
	return IsTerminalStage(deployment.Status.Stage)
}

// IsTerminalStage returns whether the stage is terminal
func IsTerminalStage(stage types.DeploymentStage) bool {
	switch stage {
	case types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		types.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
		types.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		types.DEPLOYMENT_STAGE_ROLLBACK_FAILED,
		types.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		types.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		return true
	default:
		return false
	}
}

// IsEmergencyRollout returns whether this is an emergency rollout
func IsEmergencyRollout(deployment types.Deployment) bool {
	// Check if the deployment strategy has a blast update with jira link
	if deployment.Spec.Strategy == nil || deployment.Spec.Strategy.Blast == nil {
		return false
	}
	return deployment.Spec.Strategy.Blast.JiraLink != ""
}

// ShouldSkipRollout returns whether rollout should be skipped
func ShouldSkipRollout(deployment types.Deployment) bool {
	// Never skip rollout in our simplified version
	return false
}
