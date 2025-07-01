package common

import (
	"context"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ShouldCleanup determines if the deployment should be cleaned up
func ShouldCleanup(deployment v2pb.Deployment) bool {
	return !deployment.GetDeletionTimestamp().IsZero()
}

// ShouldRollback determines if the deployment should be rolled back
func ShouldRollback(deployment v2pb.Deployment) bool {
	return deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
}

// RolloutInProgress determines if a rollout is currently in progress
func RolloutInProgress(deployment v2pb.Deployment) bool {
	switch deployment.Status.Stage {
	case v2pb.DEPLOYMENT_STAGE_VALIDATION,
		v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION,
		v2pb.DEPLOYMENT_STAGE_PLACEMENT:
		return true
	default:
		return false
	}
}

// TriggerNewRollout determines if a new rollout should be triggered
func TriggerNewRollout(deployment v2pb.Deployment) bool {
	if deployment.Spec.DesiredRevision == nil {
		return false
	}
	
	// New deployment
	if deployment.Status.CurrentRevision == nil {
		return true
	}
	
	// Desired revision changed
	return deployment.Spec.DesiredRevision.Name != deployment.Status.CurrentRevision.Name
}

// InSteadyState determines if the deployment is in steady state
func InSteadyState(deployment v2pb.Deployment) bool {
	return deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
}

// IsTerminalStage determines if the deployment is in a terminal stage
func IsTerminalStage(stage v2pb.DeploymentStage) bool {
	switch stage {
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:
		return true
	default:
		return false
	}
}

// IsCleanupStage determines if the current stage is a cleanup stage
func IsCleanupStage(stage v2pb.DeploymentStage) bool {
	switch stage {
	case v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		return true
	default:
		return false
	}
}

// IsCleanupCompleteStage determines if cleanup is complete
func IsCleanupCompleteStage(stage v2pb.DeploymentStage) bool {
	return stage == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
}

// IsRollbackStage determines if the current stage is a rollback stage
func IsRollbackStage(stage v2pb.DeploymentStage) bool {
	switch stage {
	case v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:
		return true
	default:
		return false
	}
}

// ShouldSkipRollout determines if rollout should be skipped
func ShouldSkipRollout(deployment v2pb.Deployment) bool {
	// Skip rollout if already in a terminal failed state
	return deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED
}

// IsEmergencyRollout determines if this is an emergency rollout
func IsEmergencyRollout(deployment v2pb.Deployment) bool {
	if deployment.Annotations != nil {
		if val, ok := deployment.Annotations["michelangelo.ai/emergency-rollout"]; ok {
			return val == "true"
		}
	}
	return false
}

// RollbackAlertsEnabled determines if rollback alerts are enabled
func RollbackAlertsEnabled(deployment v2pb.Deployment) bool {
	// For OSS, we always enable rollback alerts
	return true
}

// GetModelArtifactPath extracts the model artifact path - simplified for OSS
func GetModelArtifactPath(deployment v2pb.Deployment) string {
	if deployment.Spec.DesiredRevision != nil {
		// For OSS, we'll assume a simple naming convention
		return "s3://models/" + deployment.Spec.DesiredRevision.Name
	}
	return ""
}

// GetInferenceServerName gets the inference server name
func GetInferenceServerName(deployment v2pb.Deployment) string {
	if deployment.Spec.GetInferenceServer() != nil {
		return deployment.Spec.GetInferenceServer().Name
	}
	return ""
}

// BuildModelConfig creates a basic model configuration
func BuildModelConfig(deployment v2pb.Deployment) map[string]string {
	config := make(map[string]string)
	
	if deployment.Spec.DesiredRevision != nil {
		config["model_name"] = deployment.Spec.DesiredRevision.Name
		config["model_version"] = "latest"
		config["model_path"] = GetModelArtifactPath(deployment)
	}
	
	return config
}

// Actor represents a common interface for all deployment actors
type Actor interface {
	GetType() string
	Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
}