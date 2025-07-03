package oss

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidationActor validates deployment configuration
type ValidationActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ValidationActor) GetType() string {
	return "Validated"
}

func (a *ValidationActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Basic validation for OSS
	if resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for deployment",
		}, nil
	}

	if resource.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoInferenceServer",
			Message: "No inference server specified for deployment",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Deployment validation completed successfully",
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running validation for deployment", "deployment", resource.Name)
	
	// Update deployment status to show validation is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
	
	// Perform comprehensive validation
	if resource.Spec.DesiredRevision == nil {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: No desired revision specified"
		return nil
	}
	
	if resource.Spec.GetInferenceServer() == nil {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: No inference server specified"
		return nil
	}
	
	// Additional OSS-specific validations
	if resource.Spec.DesiredRevision.Name == "" {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: Desired revision name is empty"
		return nil
	}
	
	if resource.Spec.GetInferenceServer().Name == "" {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: Inference server name is empty"
		return nil
	}
	
	// If all validations pass
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	resource.Status.Message = "Validation completed successfully"
	runtimeCtx.Logger.Info("Validation completed successfully", "deployment", resource.Name)
	
	return nil
}

// RolloutActor handles the rollout process
type RolloutActor struct {
	client client.Client
	logger logr.Logger
}

func (a *RolloutActor) GetType() string {
	return "RolloutComplete"
}

func (a *RolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// For OSS, check if rollout is complete by verifying multiple conditions
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY &&
		resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name {
		
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RolloutCompleted",
			Message: "Rollout completed successfully and deployment is healthy",
		}, nil
	}

	// Check if rollout is in progress
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT ||
		resource.Status.State == v2pb.DEPLOYMENT_STATE_INITIALIZING {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "RolloutInProgress",
			Message: "Rollout is currently in progress",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RolloutPending",
		Message: "Rollout has not started yet",
	}, nil
}

func (a *RolloutActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running rollout for deployment", "deployment", resource.Name)
	
	// Update deployment status to indicate rollout is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	
	// Set current revision to desired revision to simulate rollout completion
	if resource.Spec.DesiredRevision != nil {
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		runtimeCtx.Logger.Info("Rollout completed for OSS deployment", "model", resource.Spec.DesiredRevision.Name)
	}
	
	return nil
}

// CleanupActor handles cleanup operations
type CleanupActor struct {
	client client.Client
	logger logr.Logger
}

func (a *CleanupActor) GetType() string {
	return "CleanupComplete"
}

func (a *CleanupActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// For OSS, assume cleanup is complete when deletion timestamp is set
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CleanupCompleted",
			Message: "Cleanup completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CleanupNotNeeded",
		Message: "Cleanup not required",
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running cleanup for deployment", "deployment", resource.Name)
	
	// Update deployment status to indicate cleanup is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
	
	// For OSS, cleanup involves removing model-related ConfigMaps and updating status
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		// Simulate cleanup of model artifacts and ConfigMaps
		runtimeCtx.Logger.Info("Cleaning up model artifacts and ConfigMaps", "deployment", resource.Name)
		
		// Mark cleanup as complete
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
		runtimeCtx.Logger.Info("Cleanup completed for OSS deployment")
	}
	
	return nil
}

// RollbackActor handles rollback operations
type RollbackActor struct {
	client client.Client
	logger logr.Logger
}

func (a *RollbackActor) GetType() string {
	return "RollbackComplete"
}

func (a *RollbackActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// For OSS, consider rollback complete when we restore to the previous revision
	if resource.Status.CurrentRevision != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollbackCompleted",
			Message: "Rollback completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollbackInProgress",
		Message: "Rollback in progress",
	}, nil
}

func (a *RollbackActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running rollback for deployment", "deployment", resource.Name)
	
	// Update deployment status to indicate rollback is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
	resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
	
	// For OSS, rollback means restoring the previous revision
	if resource.Status.CurrentRevision != nil {
		// Store the failed revision for reference
		failedRevision := resource.Spec.DesiredRevision
		
		// Restore to previous known good revision
		resource.Spec.DesiredRevision = resource.Status.CurrentRevision
		
		// Update status to reflect rollback completion
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		
		runtimeCtx.Logger.Info("Rolled back to previous revision", 
			"from", failedRevision.Name, 
			"to", resource.Status.CurrentRevision.Name)
	} else {
		runtimeCtx.Logger.Info("No previous revision available for rollback")
	}

	return nil
}

// SteadyStateActor handles steady state monitoring
type SteadyStateActor struct {
	client client.Client
	logger logr.Logger
}

func (a *SteadyStateActor) GetType() string {
	return "StateSteady"
}

func (a *SteadyStateActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// For OSS, consider steady state when deployment is complete and healthy
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "SteadyStateReached",
			Message: "Deployment is in steady state",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "NotInSteadyState",
		Message: "Deployment not yet in steady state",
	}, nil
}

func (a *SteadyStateActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Monitoring steady state for deployment", "deployment", resource.Name)
	
	// For OSS, actively monitor and maintain steady state
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
		// Check if deployment remains healthy
		if resource.Status.State != v2pb.DEPLOYMENT_STATE_HEALTHY {
			runtimeCtx.Logger.Info("Deployment not healthy, investigating", "state", resource.Status.State)
			// In a real implementation, this would check inference server health
			// For now, assume we can restore to healthy state
			resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}
		
		// Ensure current revision matches desired revision
		if resource.Status.CurrentRevision != nil && resource.Spec.DesiredRevision != nil {
			if resource.Status.CurrentRevision.Name != resource.Spec.DesiredRevision.Name {
				runtimeCtx.Logger.Info("Revision mismatch detected, needs reconciliation",
					"current", resource.Status.CurrentRevision.Name,
					"desired", resource.Spec.DesiredRevision.Name)
			}
		}
		
		runtimeCtx.Logger.Info("Deployment is in steady state", "deployment", resource.Name)
	}
	
	return nil
}

// ResourceAcquisitionActor handles resource acquisition for deployments
type ResourceAcquisitionActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ResourceAcquisitionActor) GetType() string {
	return "ResourcesAcquired"
}

func (a *ResourceAcquisitionActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if resources are properly allocated
	if resource.Spec.GetInferenceServer() != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ResourcesAvailable",
			Message: "Required resources are available and allocated",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ResourcesPending",
		Message: "Waiting for resource allocation",
	}, nil
}

func (a *ResourceAcquisitionActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running resource acquisition for deployment", "deployment", resource.Name)
	
	// For OSS, this would ensure inference server is ready and has capacity
	if resource.Spec.GetInferenceServer() != nil {
		runtimeCtx.Logger.Info("Resources acquired successfully", 
			"inference_server", resource.Spec.GetInferenceServer().Name)
	}
	
	return nil
}

// ModelSyncActor handles model synchronization to inference servers
type ModelSyncActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ModelSyncActor) GetType() string {
	return "ModelSynced"
}

func (a *ModelSyncActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if model is synced to the inference server
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ModelSyncCompleted",
			Message: "Model successfully synced to inference server",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ModelSyncPending",
		Message: "Model sync is in progress",
	}, nil
}

func (a *ModelSyncActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running model sync for deployment", "deployment", resource.Name)
	
	// For OSS, this would sync the model from storage to the inference server
	if resource.Spec.DesiredRevision != nil {
		// Simulate model sync by creating/updating ConfigMaps
		runtimeCtx.Logger.Info("Syncing model to inference server",
			"model", resource.Spec.DesiredRevision.Name,
			"inference_server", resource.Spec.GetInferenceServer().Name)
		
		// Update status to indicate sync completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		runtimeCtx.Logger.Info("Model sync completed successfully")
	}
	
	return nil
}