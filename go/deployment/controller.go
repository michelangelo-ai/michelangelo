package deployment

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	_defaultRequeuePeriod         = 20 * time.Minute
	_reconciliationTimeout        = 60 * time.Second
	_deploymentCleanedUpFinalizer = "deployments.michelangelo.ai/finalizer"
	_deploymentKey                = "deployment"
	_maximumConcurrentReconciles  = 5
	_modelHealthCheckTimeout      = 10 * time.Minute // Configurable timeout for model health checks
	_modelHealthCheckInterval     = 30 * time.Second // Interval between health check retries
)

// Reconciler handles deployment orchestration through plugin pattern
type Reconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Plugin   plugins.Plugin
}

// Reconcile handles deployment reconciliation using basic OSS approach
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues(_deploymentKey, req.NamespacedName.String())
	ctx, cancel := context.WithTimeout(ctx, _reconciliationTimeout)
	defer cancel()

	// Handle panics gracefully
	defer func() {
		if err := recover(); err != nil {
			logger.Error(fmt.Errorf("%+v", err), "panic occurred during deployment reconcile")
		}
	}()

	// Get deployment resource
	var deployment v2pb.Deployment
	if err := r.Get(ctx, req.NamespacedName, &deployment); err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("deployment resource not found, ignoring deletion")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to retrieve deployment object")
		return ctrl.Result{}, err
	}

	// Add logging context
	logger = r.addLoggingContext(logger, &deployment)

	originalDeployment := deployment.DeepCopy()
	result, err := r.reconcile(ctx, logger, &deployment, originalDeployment)
	if err != nil {
		logger.Error(err, "Failed to process deployment")
		return result, err
	}

	// Update deployment resource if changed
	if !reflect.DeepEqual(originalDeployment, &deployment) {
		// Save the current status before update
		currentStatus := deployment.Status

		if err := r.Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to update deployment resource")
			return result, err
		}

		// Restore and update status with the current (modified) status
		deployment.Status = currentStatus
		if err := r.Status().Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to update deployment status")
			return result, err
		}

		logger.Info("Successfully updated deployment and status",
			"stage", deployment.Status.Stage,
			"state", deployment.Status.State,
			"currentRevision", func() string {
				if deployment.Status.CurrentRevision != nil {
					return deployment.Status.CurrentRevision.Name
				}
				return ""
			}())
	}

	return result, nil
}

// reconcile contains the main reconciliation logic for OSS deployments
func (r *Reconciler) reconcile(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment, originalDeployment *v2pb.Deployment) (ctrl.Result, error) {
	defaultResult := ctrl.Result{
		Requeue:      true,
		RequeueAfter: _defaultRequeuePeriod,
	}

	// Handle finalizer
	if deployment.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(deployment, _deploymentCleanedUpFinalizer) {
			controllerutil.AddFinalizer(deployment, _deploymentCleanedUpFinalizer)
			return defaultResult, nil
		}
	}

	// Handle deletion/cleanup
	if !deployment.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("Processing deployment deletion")
		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS

		err := r.handleCleanup(ctx, logger, deployment)
		if err != nil {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED
			deployment.Status.Message = fmt.Sprintf("Cleanup failed: %v", err)
			return defaultResult, err
		}

		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
		deployment.Status.Message = "Cleanup completed successfully"
		controllerutil.RemoveFinalizer(deployment, _deploymentCleanedUpFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle rollback for failed deployments
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED && r.shouldTriggerRollback(deployment) {
		logger.Info("Triggering automatic rollback for failed deployment")
		err := r.handleRollback(ctx, logger, deployment)
		if err != nil {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED
			deployment.Status.Message = fmt.Sprintf("Rollback failed: %v", err)
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
			return defaultResult, err
		}
		return defaultResult, nil
	}

	// Handle new rollout
	if r.shouldTriggerNewRollout(deployment) {
		logger.Info("detected new rollout")
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision
		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION

		rolloutComplete, err := r.handleRollout(ctx, logger, deployment)
		if err != nil {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED
			deployment.Status.Message = fmt.Sprintf("Rollout failed: %v", err)
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
			return defaultResult, err
		}

		// Only mark as complete if rollout actually finished successfully
		if rolloutComplete {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
			deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
			deployment.Status.Message = "Rollout completed successfully"
		} else {
			// Rollout is in progress (actors blocked/waiting)
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
			deployment.Status.Message = "Rollout in progress - waiting for health checks"
		}
	}

	return defaultResult, nil
}

// shouldTriggerNewRollout determines if a new rollout should be triggered
func (r *Reconciler) shouldTriggerNewRollout(deployment *v2pb.Deployment) bool {
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

// shouldTriggerRollback determines if a rollback should be triggered for failed deployments
func (r *Reconciler) shouldTriggerRollback(deployment *v2pb.Deployment) bool {
	// Only rollback if we have a previous revision to rollback to
	if deployment.Status.CurrentRevision == nil {
		return false
	}

	// Only rollback if the deployment is in a failed state
	if deployment.Status.Stage != v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED {
		return false
	}

	// Don't rollback if we're already in a rollback state
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS ||
		deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE ||
		deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED {
		return false
	}

	return true
}

// handleRollout handles the rollout process using enhanced plugin system
// Returns (rolloutComplete, error) where rolloutComplete indicates if all actors finished successfully
func (r *Reconciler) handleRollout(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) (bool, error) {
	logger.Info("Processing deployment rollout with enhanced plugin system",
		"desiredRevision", deployment.Spec.DesiredRevision.Name,
		"inferenceServer", deployment.Spec.GetInferenceServer().Name)

	// Use enhanced plugin system for rollout
	if r.Plugin == nil {
		return false, fmt.Errorf("plugin not initialized")
	}

	// Get rollout plugin from the main plugin
	rolloutPlugin, err := r.Plugin.GetRolloutPlugin(ctx, deployment)
	if err != nil {
		return false, fmt.Errorf("failed to get rollout plugin: %w", err)
	}

	// Get actors from rollout plugin
	actors := rolloutPlugin.GetActors()
	logger.Info("Running enhanced rollout with actors", "actorCount", len(actors))

	// Execute all actors in sequence following Uber pattern
	runtimeCtx := plugins.RequestContext{Logger: logger}
	for _, actor := range actors {
		actorType := actor.GetType()
		logger.Info("Executing actor", "actorType", actorType)

		// Retrieve condition
		var existingCondition *apipb.Condition
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == actorType {
				existingCondition = condition
				break
			}
		}

		if existingCondition == nil {
			existingCondition = &apipb.Condition{
				Type:   actorType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			}
		}

		// Retrieve current state
		updatedCondition, err := actor.Retrieve(ctx, runtimeCtx, deployment, existingCondition)
		if err != nil {
			logger.Error(err, "Actor retrieve failed", "actorType", actorType)
			return false, fmt.Errorf("actor %s retrieve failed: %w", actorType, err)
		}

		// If condition is not TRUE, run the actor
		if updatedCondition.Status != apipb.CONDITION_STATUS_TRUE {
			logger.Info("Running actor", "actorType", actorType, "reason", updatedCondition.Reason)

			err = actor.Run(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Actor run failed", "actorType", actorType)
				return false, fmt.Errorf("actor %s run failed: %w", actorType, err)
			}

			// Re-retrieve after running
			updatedCondition, err = actor.Retrieve(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Actor re-retrieve failed", "actorType", actorType)
				return false, fmt.Errorf("actor %s re-retrieve failed: %w", actorType, err)
			}
		}

		// Update condition in deployment status
		found := false
		for i, condition := range deployment.Status.Conditions {
			if condition.Type == actorType {
				deployment.Status.Conditions[i] = updatedCondition
				found = true
				break
			}
		}

		if !found {
			deployment.Status.Conditions = append(deployment.Status.Conditions, updatedCondition)
		}

		logger.Info("Actor completed", "actorType", actorType, "status", updatedCondition.Status, "message", updatedCondition.Message)

		// Stop execution if this actor failed - do not proceed to next actors
		if updatedCondition.Status != apipb.CONDITION_STATUS_TRUE {
			logger.Info("Actor not ready, stopping rollout execution", "actorType", actorType, "status", updatedCondition.Status)
			return false, nil // Controller will requeue and retry later
		}
	}

	logger.Info("Enhanced rollout plugin execution completed")
	return true, nil
}

// handleRollback handles the rollback process using Uber's plugin system pattern
func (r *Reconciler) handleRollback(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Processing deployment rollback using plugin system",
		"currentRevision", func() string {
			if deployment.Status.CurrentRevision != nil {
				return deployment.Status.CurrentRevision.Name
			}
			return "none"
		}(),
		"candidateRevision", func() string {
			if deployment.Status.CandidateRevision != nil {
				return deployment.Status.CandidateRevision.Name
			}
			return "none"
		}())

	// Use plugin system for rollback
	if r.Plugin == nil {
		return fmt.Errorf("plugin not initialized")
	}

	// Get rollback plugin from the main plugin
	rollbackPlugin := r.Plugin.GetRollbackPlugin()
	if rollbackPlugin == nil {
		return fmt.Errorf("rollback plugin not available")
	}

	// Get actors from rollback plugin
	actors := rollbackPlugin.GetActors()
	logger.Info("Running rollback with actors", "actorCount", len(actors))

	// Execute all rollback actors in sequence following Uber pattern
	runtimeCtx := plugins.RequestContext{Logger: logger}
	for _, actor := range actors {
		actorType := actor.GetType()
		logger.Info("Executing rollback actor", "actorType", actorType)

		// Retrieve condition
		var existingCondition *apipb.Condition
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == actorType {
				existingCondition = condition
				break
			}
		}

		if existingCondition == nil {
			existingCondition = &apipb.Condition{
				Type:   actorType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			}
		}

		// Retrieve current state
		updatedCondition, err := actor.Retrieve(ctx, runtimeCtx, deployment, existingCondition)
		if err != nil {
			logger.Error(err, "Rollback actor retrieve failed", "actorType", actorType)
			return fmt.Errorf("rollback actor %s retrieve failed: %w", actorType, err)
		}

		// If condition is not TRUE, run the actor
		if updatedCondition.Status != apipb.CONDITION_STATUS_TRUE {
			logger.Info("Running rollback actor", "actorType", actorType, "reason", updatedCondition.Reason)

			err = actor.Run(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Rollback actor run failed", "actorType", actorType)
				deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED
				deployment.Status.Message = fmt.Sprintf("Rollback actor %s failed: %v", actorType, err)
				return fmt.Errorf("rollback actor %s run failed: %w", actorType, err)
			}

			// Re-retrieve after running
			updatedCondition, err = actor.Retrieve(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Rollback actor re-retrieve failed", "actorType", actorType)
				return fmt.Errorf("rollback actor %s re-retrieve failed: %w", actorType, err)
			}
		}

		// Update condition in deployment status
		found := false
		for i, condition := range deployment.Status.Conditions {
			if condition.Type == actorType {
				deployment.Status.Conditions[i] = updatedCondition
				found = true
				break
			}
		}

		if !found {
			deployment.Status.Conditions = append(deployment.Status.Conditions, updatedCondition)
		}

		logger.Info("Rollback actor completed", "actorType", actorType, "status", updatedCondition.Status, "message", updatedCondition.Message)

		// Stop execution if this actor failed
		if updatedCondition.Status != apipb.CONDITION_STATUS_TRUE {
			logger.Info("Rollback actor not ready, will retry", "actorType", actorType, "status", updatedCondition.Status)
			return fmt.Errorf("rollback actor %s not ready: %s", actorType, updatedCondition.Message)
		}
	}

	// All rollback actors completed successfully
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
	deployment.Status.Message = "Rollback completed successfully"

	logger.Info("Rollback plugin execution completed successfully")
	return nil
}

// handleCleanup handles the cleanup process for OSS deployments
func (r *Reconciler) handleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Processing deployment cleanup")

	// Use the plugin system to handle cleanup
	if r.Plugin != nil {
		logger.Info("Delegating cleanup to deployment plugin")
		err := r.Plugin.HandleCleanup(ctx, logger, deployment)
		if err != nil {
			logger.Error(err, "Plugin cleanup failed")
			return err
		}
	}

	logger.Info("Cleanup completed for OSS deployment")
	return nil
}

// addLoggingContext adds contextual logging information
func (r *Reconciler) addLoggingContext(logger logr.Logger, deployment *v2pb.Deployment) logr.Logger {
	logger = logger.WithValues("desiredModel", "")
	logger = logger.WithValues("candidateModel", "")
	logger = logger.WithValues("currentModel", "")

	if deployment.Spec.DesiredRevision != nil {
		logger = logger.WithValues("desiredModel", deployment.Spec.DesiredRevision.Name)
	}
	if deployment.Status.CandidateRevision != nil {
		logger = logger.WithValues("candidateModel", deployment.Status.CandidateRevision.Name)
	}
	if deployment.Status.CurrentRevision != nil {
		logger = logger.WithValues("currentModel", deployment.Status.CurrentRevision.Name)
	}

	return logger
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Log = mgr.GetLogger().WithName(_deploymentKey)
	r.Recorder = mgr.GetEventRecorderFor(_deploymentKey)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Deployment{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: _maximumConcurrentReconciles}).
		Complete(r)
}
