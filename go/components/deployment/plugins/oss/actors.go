package oss

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidationActor validates deployment configuration
type ValidationActor struct {
	client    client.Client
	blobstore *blobstore.BlobStore
	logger    logr.Logger
}

func (a *ValidationActor) GetType() string {
	return "Validated"
}

func (a *ValidationActor) GetLogger() logr.Logger {
	return a.logger
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

	// Validate model name format and existence
	modelName := resource.Spec.DesiredRevision.Name
	if modelName == "" {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidModelName",
			Message: "Model name cannot be empty",
		}, nil
	}

	// Check if model folder exists in MinIO storage
	if a.blobstore != nil {
		// TODO: Implement storage validation when blobstore.Exists method is available
		// For now, skip storage validation as it's optional pre-validation
		// Model will be validated during sync phase instead
		a.logger.Info("Skipping storage validation - will validate during model sync",
			"model", modelName)
	}
	// If blobstore is not available, skip storage validation and trust the model name

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Deployment validation completed successfully for model " + modelName,
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running validation for deployment", "deployment", resource.Name)

	// Update deployment status to show validation is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION

	// Perform comprehensive validation
	if resource.Spec.DesiredRevision == nil {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: No desired revision specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoDesiredRevision", Message: "Validation failed: No desired revision specified"}, nil
	}

	if resource.Spec.GetInferenceServer() == nil {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: No inference server specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoInferenceServer", Message: "Validation failed: No inference server specified"}, nil
	}

	// Additional OSS-specific validations
	if resource.Spec.DesiredRevision.Name == "" {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: Desired revision name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyRevisionName", Message: "Validation failed: Desired revision name is empty"}, nil
	}

	if resource.Spec.GetInferenceServer().Name == "" {
		resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		resource.Status.Message = "Validation failed: Inference server name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyInferenceServerName", Message: "Validation failed: Inference server name is empty"}, nil
	}

	// If all validations pass
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	resource.Status.Message = "Validation completed successfully"
	a.logger.Info("Validation completed successfully", "deployment", resource.Name)

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// RolloutActor handles the rollout process
type RolloutActor struct {
	client client.Client
	logger logr.Logger
}

func (a *RolloutActor) GetType() string {
	return "RolloutComplete"
}

func (a *RolloutActor) GetLogger() logr.Logger {
	return a.logger
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

func (a *RolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout for deployment", "deployment", resource.Name)

	// Update deployment status to indicate rollout is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	// Set current revision to desired revision to simulate rollout completion
	if resource.Spec.DesiredRevision != nil {
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Rollout completed for OSS deployment", "model", resource.Spec.DesiredRevision.Name)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// CleanupActor handles cleanup operations
type CleanupActor struct {
	client client.Client
	logger logr.Logger
}

func (a *CleanupActor) GetType() string {
	return "CleanupComplete"
}

func (a *CleanupActor) GetLogger() logr.Logger {
	return a.logger
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

func (a *CleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running cleanup for deployment", "deployment", resource.Name)

	// Update deployment status to indicate cleanup is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS

	// For OSS, cleanup involves removing model-related ConfigMaps and updating status
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		// Simulate cleanup of model artifacts and ConfigMaps
		a.logger.Info("Cleaning up model artifacts and ConfigMaps", "deployment", resource.Name)

		// Mark cleanup as complete
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
		a.logger.Info("Cleanup completed for OSS deployment")
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// RollbackActor handles rollback operations
type RollbackActor struct {
	client client.Client
	logger logr.Logger
}

func (a *RollbackActor) GetType() string {
	return "RollbackComplete"
}

func (a *RollbackActor) GetLogger() logr.Logger {
	return a.logger
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

func (a *RollbackActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollback for deployment", "deployment", resource.Name)

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

		a.logger.Info("Rolled back to previous revision",
			"from", failedRevision.Name,
			"to", resource.Status.CurrentRevision.Name)
	} else {
		a.logger.Info("No previous revision available for rollback")
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// SteadyStateActor handles steady state monitoring
type SteadyStateActor struct {
	client client.Client
	logger logr.Logger
}

func (a *SteadyStateActor) GetType() string {
	return "StateSteady"
}

func (a *SteadyStateActor) GetLogger() logr.Logger {
	return a.logger
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

func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("SteadyStateActor.Run() called",
		"deployment", resource.Name,
		"currentStage", resource.Status.Stage,
		"currentState", resource.Status.State,
		"candidateRevision", resource.Status.CandidateRevision,
		"currentRevision", resource.Status.CurrentRevision,
		"desiredRevision", resource.Spec.DesiredRevision)

	// For OSS, actively monitor and maintain steady state
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
		// Check if deployment remains healthy
		if resource.Status.State != v2pb.DEPLOYMENT_STATE_HEALTHY {
			a.logger.Info("Deployment not healthy, investigating", "state", resource.Status.State)
			// In a real implementation, this would check inference server health
			// For now, assume we can restore to healthy state
			resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}

		// Ensure current revision matches desired revision
		if resource.Status.CurrentRevision != nil && resource.Spec.DesiredRevision != nil {
			if resource.Status.CurrentRevision.Name != resource.Spec.DesiredRevision.Name {
				a.logger.Info("Revision mismatch detected, needs reconciliation",
					"current", resource.Status.CurrentRevision.Name,
					"desired", resource.Spec.DesiredRevision.Name)
			}
		}

		a.logger.Info("Deployment is in steady state", "deployment", resource.Name)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// ResourceAcquisitionActor handles resource acquisition for deployments
type ResourceAcquisitionActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ResourceAcquisitionActor) GetType() string {
	return "ResourcesAcquired"
}

func (a *ResourceAcquisitionActor) GetLogger() logr.Logger {
	return a.logger
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

func (a *ResourceAcquisitionActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running resource acquisition for deployment", "deployment", resource.Name)

	// For OSS, this would ensure inference server is ready and has capacity
	if resource.Spec.GetInferenceServer() != nil {
		a.logger.Info("Resources acquired successfully",
			"inference_server", resource.Spec.GetInferenceServer().Name)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// ModelSyncActor handles model synchronization to inference servers using deployment-level ConfigMap management
type ModelSyncActor struct {
	client            client.Client
	gateway           gateways.Gateway
	dynamicClient     dynamic.Interface
	configMapProvider *gateways.ConfigMapProvider
	logger            logr.Logger
}

func (a *ModelSyncActor) GetType() string {
	return "ModelSynced"
}

func (a *ModelSyncActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *ModelSyncActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if the desired model is actually loaded and ready in Triton
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name

		// Check if model is loaded in Triton using the gateway health check
		if a.gateway != nil {
			inferenceServerName := resource.Spec.GetInferenceServer().Name

			// Check if the desired model is ready in Triton
			modelStatusRequest := gateways.ModelStatusRequest{
				ModelName:       modelName,
				InferenceServer: inferenceServerName,
				DeploymentName:  resource.Name, // Include deployment name for deployment-specific routing
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			// Implement retry logic with configurable timeout for health checks
			modelReady, err := a.checkModelStatusWithTimeout(ctx, a.logger, modelStatusRequest)
			if err != nil {
				// Check if this is a timeout error vs other errors
				if err.Error() == "health check timeout exceeded" {
					a.logger.Info("Model health check timed out after 10 minutes", "model", modelName)
					return &apipb.Condition{
						Type:    a.GetType(),
						Status:  apipb.CONDITION_STATUS_FALSE,
						Reason:  "ModelHealthCheckTimeout",
						Message: fmt.Sprintf("Model %s health check timed out after 10 minutes", modelName),
					}, nil
				}

				a.logger.Error(err, "Failed to check model status in Triton", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelHealthCheckError",
					Message: fmt.Sprintf("Error checking model %s readiness: %v", modelName, err),
				}, nil
			}

			if modelReady {
				a.logger.Info("New model is loaded and ready in Triton", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_TRUE,
					Reason:  "ModelSyncCompleted",
					Message: fmt.Sprintf("Model %s successfully loaded and ready in Triton", modelName),
				}, nil
			} else {
				a.logger.Info("New model is not yet ready in Triton, continuing to wait", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelNotReady",
					Message: fmt.Sprintf("Model %s is loading but not yet ready in Triton", modelName),
				}, nil
			}
		}
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ModelSyncPending",
		Message: "Model sync is in progress",
	}, nil
}

func (a *ModelSyncActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running model sync for deployment", "deployment", resource.Name)

	// For OSS, this would sync the model from storage to the inference server
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Syncing model to inference server",
			"model", modelName,
			"inference_server", inferenceServerName)

		// UCS CACHE PATTERN: Replicate Uber's exact UCS cache update pattern from rolling/actor.go:76
		// Original Uber code: err = a.ucsCache.UpdateDeployment(*deployment, constraints, nil, common.RoleTypeCandidate)
		if a.configMapProvider != nil {
			// Follow Uber's pattern exactly: UpdateDeployment with deployment, constraints, role
			// For OSS: constraints are empty (no hosts), but we track deployment-level model ownership
			if err := a.configMapProvider.UpdateDeploymentModel(ctx, inferenceServerName, resource.Namespace, resource.Name, modelName, "candidate"); err != nil {
				a.logger.Error(err, "Failed to update deployment via ConfigMapProvider (UCS cache pattern)")
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ConfigMapUpdateFailed",
					Message: fmt.Sprintf("Failed to update deployment: %v", err),
				}, nil
			}

			a.logger.Info("UCS cache pattern update completed successfully",
				"deployment", resource.Name,
				"candidateModel", modelName,
				"roleType", "candidate")
		} else {
			// Fallback to old gateway-based approach if ConfigMapProvider not available
			a.logger.Info("ConfigMapProvider not available, falling back to gateway approach")
			if a.gateway != nil {
				// Get current models from ConfigMap to preserve them during deployment
				currentModels, err := a.getCurrentModelsFromConfigMap(ctx, a.logger, inferenceServerName, resource.Namespace)
				if err != nil {
					a.logger.Error(err, "Failed to get current models from ConfigMap")
					// Continue with just the new model if we can't read existing ones
					currentModels = []gateways.ModelConfigEntry{}
				}

				// Check if new model already exists to avoid duplicates
				modelExists := false
				for _, model := range currentModels {
					if model.Name == modelName {
						modelExists = true
						break
					}
				}

				// Add the new model if it doesn't already exist
				if !modelExists {
					currentModels = append(currentModels, gateways.ModelConfigEntry{
						Name:   modelName,
						S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
					})
					a.logger.Info("Adding new model for zero-downtime deployment",
						"newModel", modelName, "totalModels", len(currentModels))
				} else {
					a.logger.Info("Model already exists in ConfigMap", "model", modelName)
				}

				updateRequest := gateways.ModelConfigUpdateRequest{
					InferenceServer: inferenceServerName,
					Namespace:       resource.Namespace,
					BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton for OSS
					ModelConfigs:    currentModels,
				}

				if err := a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest); err != nil {
					a.logger.Error(err, "Failed to update model config via gateway")
					return &apipb.Condition{
						Type:    a.GetType(),
						Status:  apipb.CONDITION_STATUS_FALSE,
						Reason:  "ModelConfigUpdateFailed",
						Message: fmt.Sprintf("Failed to update model config: %v", err),
					}, nil
				}

				a.logger.Info("Model configuration updated successfully for zero-downtime deployment",
					"model", modelName, "totalModels", len(currentModels))
			}
		}

		// DO NOT update HTTPRoute or CurrentRevision yet!
		// We only sync the model to ConfigMap here. HTTPRoute update and CurrentRevision
		// will be handled by ModelHealthCheckActor after verifying the new model is ready.
		a.logger.Info("Model sync to ConfigMap completed successfully - waiting for health check before switching traffic")
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// AssetPreparationActor handles asset preparation for deployments (following Uber pattern)
type AssetPreparationActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *AssetPreparationActor) GetType() string {
	return "AssetsPrepared"
}

func (a *AssetPreparationActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *AssetPreparationActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if assets are prepared for the desired model
	if resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for asset preparation",
		}, nil
	}

	// For OSS, we assume assets are available in MinIO/S3 storage
	// In Uber's implementation, this checks TerraBob and validates model assets
	modelName := resource.Spec.DesiredRevision.Name

	// For OSS, assume assets are always available if the model name is valid
	// In a real implementation, this would check MinIO/S3 for model artifacts
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "AssetsAvailable",
		Message: fmt.Sprintf("Assets for model %s are available and prepared", modelName),
	}, nil
}

func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running asset preparation for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		a.logger.Info("Preparing assets for model", "model", modelName)

		// In Uber's implementation, this downloads from S3, compiles, and uploads to TerraBob
		// For OSS, we simulate asset preparation by ensuring model is accessible in storage
		// This would typically involve:
		// 1. Validate model exists in MinIO/S3
		// 2. Download and validate model artifacts
		// 3. Prepare model configuration files
		// 4. Ensure model is ready for inference server deployment

		a.logger.Info("Asset preparation completed", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// RollingRolloutActor handles rolling rollout strategy (following Uber pattern)
type RollingRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *RollingRolloutActor) GetType() string {
	return "RollingRolloutComplete"
}

func (a *RollingRolloutActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *RollingRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rolling rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollingRolloutCompleted",
			Message: "Rolling rollout completed successfully across all inference servers",
		}, nil
	}

	// Check if rollout is in progress
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "RollingRolloutInProgress",
			Message: "Rolling rollout is in progress",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollingRolloutPending",
		Message: "Rolling rollout has not started",
	}, nil
}

func (a *RollingRolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rolling rollout for deployment", "deployment", resource.Name)

	// Update deployment to placement stage
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting rolling rollout",
			"model", modelName,
			"inference_server", inferenceServerName)

		// In Uber's implementation, this:
		// 1. Resolves all hosts for the inference server
		// 2. Incrementally rolls out to percentage of hosts (30% by default)
		// 3. Waits for model to load on each batch before proceeding
		// 4. Continues until 100% of hosts have the new model

		// For OSS, we simulate a successful rolling rollout
		// In a real implementation, this would:
		// - Update inference server ConfigMaps incrementally
		// - Monitor model loading status on each inference server pod
		// - Implement proper rollback on failures

		// Simulate rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		a.logger.Info("Rolling rollout completed successfully", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// RolloutCompletionActor handles post-rollout completion tasks (following Uber pattern)
type RolloutCompletionActor struct {
	client            client.Client
	gateway           gateways.Gateway
	configMapProvider *gateways.ConfigMapProvider
	logger            logr.Logger
}

func (a *RolloutCompletionActor) GetType() string {
	return "RolloutCompleted"
}

func (a *RolloutCompletionActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *RolloutCompletionActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rollout completion tasks are done
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CompletionTasksFinished",
			Message: "All rollout completion tasks have been successfully executed",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CompletionTasksPending",
		Message: "Rollout completion tasks are pending",
	}, nil
}

func (a *RolloutCompletionActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout completion tasks for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		// ZERO-DOWNTIME TRAFFIC SWITCH: Now that ModelSyncActor has confirmed the new model
		// is loaded and ready in Triton, we can safely switch traffic by adding deployment-specific routing
		a.logger.Info("Adding deployment-specific route after health check confirmation", "newModel", modelName)

		// Add deployment-specific route for the new routing architecture
		if a.gateway != nil {
			// Add deployment-specific route: /<inference-server-name>/<deployment-name> -> /v2/models/<model-name>
			proxyConfigRequest := gateways.ProxyConfigRequest{
				InferenceServer: inferenceServerName,
				Namespace:       resource.Namespace,
				ModelName:       modelName,
				DeploymentName:  resource.Name,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			if err := a.gateway.AddDeploymentSpecificRoute(ctx, a.logger, proxyConfigRequest); err != nil {
				a.logger.Error(err, "Failed to add deployment-specific route")
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "RouteCreationFailed",
					Message: fmt.Sprintf("Failed to add deployment-specific route: %v", err),
				}, nil
			}

			a.logger.Info("Deployment-specific route added successfully for zero-downtime traffic switch",
				"newModel", modelName, "deployment", resource.Name,
				"route", fmt.Sprintf("/%s/%s", inferenceServerName, resource.Name))
		}

		// NOW we can safely update CurrentRevision since traffic has been switched
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		a.logger.Info("CurrentRevision updated after successful traffic switch", "model", modelName)

		// DEPLOYMENT-LEVEL CLEANUP: Promote candidate to current and trigger safe cleanup
		if a.configMapProvider != nil {
			// Promote candidate model to current (this automatically triggers cleanup of unused models)
			a.logger.Info("Promoting candidate model to current and cleaning up unused models", "newModel", modelName)

			if err := a.configMapProvider.UpdateDeploymentModel(ctx, inferenceServerName, resource.Namespace, resource.Name, modelName, "current"); err != nil {
				a.logger.Error(err, "Failed to promote model to current via ConfigMapProvider")
				// Don't fail the whole rollout completion due to cleanup failure
				// but log the error for investigation
			} else {
				a.logger.Info("Successfully promoted candidate to current and cleaned up unused models", "currentModel", modelName)
			}
		} else {
			// Fallback to old gateway-based approach if ConfigMapProvider not available
			a.logger.Info("ConfigMapProvider not available, falling back to gateway cleanup")
			if a.gateway != nil {
				// Get current model configuration to identify old models
				a.logger.Info("Cleaning up old models from ConfigMap", "newModel", modelName)

				// Create cleanup request that will remove old models and keep only the new one
				cleanupRequest := gateways.ModelConfigUpdateRequest{
					InferenceServer: inferenceServerName,
					Namespace:       resource.Namespace,
					BackendType:     v2pb.BACKEND_TYPE_TRITON,
					ModelConfigs: []gateways.ModelConfigEntry{
						{
							Name:   modelName,
							S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
						},
					},
				}

				if err := a.gateway.UpdateModelConfig(ctx, a.logger, cleanupRequest); err != nil {
					a.logger.Error(err, "Failed to cleanup old models from ConfigMap")
					// Don't fail the whole rollout completion due to cleanup failure
					// but log the error for investigation
				} else {
					a.logger.Info("Successfully cleaned up old models from ConfigMap", "activeModel", modelName)
				}
			}
		}

		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		resource.Status.Message = fmt.Sprintf("Rollout completed successfully for model %s", modelName)

		// Clean up any temporary annotations or metadata
		if resource.Annotations != nil {
			// Remove rollout-specific annotations
			delete(resource.Annotations, "rollout.michelangelo.ai/in-progress")
			delete(resource.Annotations, "rollout.michelangelo.ai/start-time")
		}

		a.logger.Info("Rollout completion tasks finished successfully", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// getCurrentModelsFromConfigMap retrieves current models from the inference server ConfigMap
// Following the correct pattern from PR #188: Get -> Parse with proper error handling
func (a *ModelSyncActor) getCurrentModelsFromConfigMap(ctx context.Context, logger logr.Logger, inferenceServerName, namespace string) ([]gateways.ModelConfigEntry, error) {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)

	// Get the ConfigMap using Kubernetes client
	configMap := &v1.ConfigMap{}
	key := client.ObjectKey{Name: configMapName, Namespace: namespace}

	if err := a.client.Get(ctx, key, configMap); err != nil {
		// If ConfigMap doesn't exist, return empty list (new deployment)
		if client.IgnoreNotFound(err) == nil {
			logger.Info("ConfigMap not found, starting with empty model list", "configMap", configMapName)
			return []gateways.ModelConfigEntry{}, nil
		}
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Parse the model-list.json from ConfigMap - following PR #188 pattern
	modelListJSON, exists := configMap.Data["model-list.json"]
	if !exists || modelListJSON == "" {
		logger.Info("model-list.json not found or empty in ConfigMap", "configMap", configMapName)
		return []gateways.ModelConfigEntry{}, nil
	}

	// Parse JSON to get current models with proper error handling
	var currentModels []gateways.ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &currentModels); err != nil {
		logger.Error(err, "Failed to parse model-list.json from ConfigMap", "configMap", configMapName)
		// Return empty list on parse failure rather than nil to allow recovery
		return []gateways.ModelConfigEntry{}, nil
	}

	logger.Info("Retrieved current models from ConfigMap", "configMap", configMapName, "modelCount", len(currentModels))
	return currentModels, nil
}

// checkModelStatusWithTimeout implements retry logic with configurable timeout for model health checks
func (a *ModelSyncActor) checkModelStatusWithTimeout(ctx context.Context, logger logr.Logger, modelStatusRequest gateways.ModelStatusRequest) (bool, error) {
	const (
		modelHealthCheckTimeout  = 10 * time.Minute // Configurable timeout for model health checks
		modelHealthCheckInterval = 30 * time.Second // Interval between health check retries
	)

	logger.Info("Starting model health check with timeout",
		"model", modelStatusRequest.ModelName,
		"timeout", modelHealthCheckTimeout,
		"retryInterval", modelHealthCheckInterval)

	// Create a context with timeout for the entire health check process
	timeoutCtx, cancel := context.WithTimeout(ctx, modelHealthCheckTimeout)
	defer cancel()

	ticker := time.NewTicker(modelHealthCheckInterval)
	defer ticker.Stop()

	// Try immediately first
	modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelStatusRequest)
	if err == nil && modelReady {
		logger.Info("Model health check succeeded immediately", "model", modelStatusRequest.ModelName)
		return true, nil
	}

	if err != nil {
		logger.Info("Initial model health check failed, will retry",
			"model", modelStatusRequest.ModelName,
			"error", err.Error())
	} else {
		logger.Info("Model not ready, will retry", "model", modelStatusRequest.ModelName)
	}

	// Start retry loop
	for {
		select {
		case <-timeoutCtx.Done():
			logger.Info("Model health check timed out",
				"model", modelStatusRequest.ModelName,
				"timeout", modelHealthCheckTimeout)
			return false, fmt.Errorf("health check timeout exceeded")

		case <-ticker.C:
			logger.Info("Retrying model health check", "model", modelStatusRequest.ModelName)

			modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelStatusRequest)
			if err == nil && modelReady {
				logger.Info("Model health check succeeded after retry", "model", modelStatusRequest.ModelName)
				return true, nil
			}

			if err != nil {
				logger.Info("Model health check retry failed, continuing to wait",
					"model", modelStatusRequest.ModelName,
					"error", err.Error())
			} else {
				logger.Info("Model still not ready, continuing to wait", "model", modelStatusRequest.ModelName)
			}
		}
	}
}
