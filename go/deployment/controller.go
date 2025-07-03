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
		if err := r.Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to update deployment resource")
			return result, err
		}

		// Update status
		deployment.Status = originalDeployment.Status
		if err := r.Status().Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to update deployment status")
			return result, err
		}
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

	// Handle new rollout
	if r.shouldTriggerNewRollout(deployment) {
		logger.Info("detected new rollout")
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision
		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
		
		err := r.handleRollout(ctx, logger, deployment)
		if err != nil {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED
			deployment.Status.Message = fmt.Sprintf("Rollout failed: %v", err)
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
			return defaultResult, err
		}
		
		// Move to completion
		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		deployment.Status.Message = "Rollout completed successfully"
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

// handleRollout handles the rollout process using enhanced plugin system
func (r *Reconciler) handleRollout(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Processing deployment rollout with enhanced plugin system", 
		"desiredRevision", deployment.Spec.DesiredRevision.Name,
		"inferenceServer", deployment.Spec.GetInferenceServer().Name)
	
	// Use enhanced plugin system for rollout
	if r.Plugin == nil {
		return fmt.Errorf("plugin not initialized")
	}
	
	// Get rollout plugin from the main plugin
	rolloutPlugin, err := r.Plugin.GetRolloutPlugin(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to get rollout plugin: %w", err)
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
			return fmt.Errorf("actor %s retrieve failed: %w", actorType, err)
		}
		
		// If condition is not TRUE, run the actor
		if updatedCondition.Status != apipb.CONDITION_STATUS_TRUE {
			logger.Info("Running actor", "actorType", actorType, "reason", updatedCondition.Reason)
			
			err = actor.Run(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Actor run failed", "actorType", actorType)
				return fmt.Errorf("actor %s run failed: %w", actorType, err)
			}
			
			// Re-retrieve after running
			updatedCondition, err = actor.Retrieve(ctx, runtimeCtx, deployment, updatedCondition)
			if err != nil {
				logger.Error(err, "Actor re-retrieve failed", "actorType", actorType)
				return fmt.Errorf("actor %s re-retrieve failed: %w", actorType, err)
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
	}
	
	logger.Info("Enhanced rollout plugin execution completed")
	return nil
}

// handleCleanup handles the cleanup process for OSS deployments
func (r *Reconciler) handleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Processing deployment cleanup")
	
	// For OSS, cleanup would involve:
	// 1. Remove ConfigMaps
	// 2. Clean up any deployment-specific resources
	
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