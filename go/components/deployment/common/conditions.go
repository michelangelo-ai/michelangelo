package common

import (
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _terminalStages = map[v2pb.DeploymentStage]bool{
	v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE:  true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE: true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE: true,
	v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED:    true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:   true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:   true,
}

var _rollbackStages = map[v2pb.DeploymentStage]bool{
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS: true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE:    true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:      true,
}

var _cleanUpStages = map[v2pb.DeploymentStage]bool{
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS: true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE:    true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:      true,
}

var _cleanUpCompleteStages = map[v2pb.DeploymentStage]bool{
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE: true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:   true,
}

const (
	_stageLabel = "stage"
)

// TriggerNewRollout ...
func TriggerNewRollout(deployment v2pb.Deployment) bool {
	desiredRevision := deployment.Spec.DesiredRevision

	// If the target and candidate aren't the same, but the stage is not terminal,
	// then it means there is an ongoing deployment. In this case, we can't trigger a new rollout.
	// We must instead wait for the rollout to hit a terminal stage.
	return !desiredRevisionEqual(desiredRevision, deployment.Status.CandidateRevision) &&
		(IsTerminalStage(deployment.Status.Stage) || isInitializationStage(deployment.Status.Stage))
}

// ShouldRollback ...
func ShouldRollback(deployment v2pb.Deployment) bool {
	desiredRevision := deployment.Spec.DesiredRevision
	candidateRevision := deployment.Status.CandidateRevision

	// If the target is not the same as the candidate, and the stage is not terminal, then we must
	// rollback the current rollout.
	return desiredRevision != nil &&
		!desiredRevisionEqual(desiredRevision, candidateRevision) &&
		!IsTerminalStage(deployment.Status.Stage) &&
		!isInitializationStage(deployment.Status.Stage)
}

// RolloutInProgress ...
func RolloutInProgress(deployment v2pb.Deployment) bool {
	currentRevision := deployment.Status.CurrentRevision
	candidateRevision := deployment.Status.CandidateRevision

	return !revisionEqual(currentRevision, candidateRevision) &&
		!IsTerminalStage(deployment.Status.Stage) &&
		!isInitializationStage(deployment.Status.Stage)
}

// InSteadyState check if current deployment needs to go through steady state plugin.
func InSteadyState(deployment v2pb.Deployment) bool {
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE ||
		deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE {
		return true
	}
	return false
}

// ShouldCleanup ...
func ShouldCleanup(deployment v2pb.Deployment) bool {
	currentRevision := deployment.Status.GetCurrentRevision()
	candidateRevision := deployment.Status.GetCandidateRevision()
	markedForDeletion := !deployment.ObjectMeta.DeletionTimestamp.IsZero()

	return markedForDeletion ||
		deployment.Spec.GetDeletionSpec().GetDeleted() ||
		(deployment.Spec.DesiredRevision == nil &&
			(currentRevision != nil || candidateRevision != nil))
}

// IsTerminalStage checks if the given stage is terminal. Exported to be used in testing.
func IsTerminalStage(stage v2pb.DeploymentStage) bool {
	_, ok := _terminalStages[stage]
	return ok
}

// IsRollbackStage checks if deployment is in rollback stage.
func IsRollbackStage(stage v2pb.DeploymentStage) bool {
	_, ok := _rollbackStages[stage]
	return ok
}

// IsCleanupStage checks if deployment is in clean up stage.
func IsCleanupStage(stage v2pb.DeploymentStage) bool {
	_, ok := _cleanUpStages[stage]
	return ok
}

// IsCleanupCompleteStage checks if deployment is in complete stage.
func IsCleanupCompleteStage(stage v2pb.DeploymentStage) bool {
	_, ok := _cleanUpCompleteStages[stage]
	return ok
}

func isInitializationStage(stage v2pb.DeploymentStage) bool {
	return stage == v2pb.DEPLOYMENT_STAGE_INVALID
}

// ShouldSkipRollout checks if current revision is already equal to candidate revision.
func ShouldSkipRollout(deployment v2pb.Deployment) bool {
	candidateRevision := deployment.Status.GetCandidateRevision()
	currentRevision := deployment.Status.GetCurrentRevision()

	return candidateRevision != nil && revisionEqual(candidateRevision, currentRevision)
}

// IsEmergencyRollout checks if the deployment strategy is of blast type
func IsEmergencyRollout(deployment v2pb.Deployment) bool {
	if strategy := deployment.Spec.GetStrategy(); strategy != nil {
		isEmergency := strategy.GetBlast()
		return isEmergency != nil
	}

	return false
}

// RollbackAlertsEnabled checks if the deployment has the rollback alerts enabled
func RollbackAlertsEnabled(deployment v2pb.Deployment) bool {
	if IsEmergencyRollout(deployment) {
		withRollbackAlerts := deployment.Spec.Strategy.GetBlast().GetWithRollbackTrigger()
		return withRollbackAlerts
	}

	return true
}

// Helper functions for revision equality since protobuf doesn't have Equal method
func revisionEqual(a, b *apipb.ResourceIdentifier) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name && a.Namespace == b.Namespace
}

func desiredRevisionEqual(a, b *apipb.ResourceIdentifier) bool {
	return revisionEqual(a, b)
}
