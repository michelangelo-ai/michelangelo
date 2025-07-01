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
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
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
	Plugin   *oss.Plugin
}

// NewReconciler creates a new deployment reconciler
func NewReconciler(client client.Client, logger logr.Logger, gateway inferenceserver.Gateway) *Reconciler {
	return &Reconciler{
		Client: client,
		Log:    logger,
		Plugin: oss.NewPlugin(gateway),
	}
}

// Reconcile handles deployment reconciliation using plugin-based approach
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

// reconcile contains the main reconciliation logic following the reference implementation pattern
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

	originalStage := deployment.Status.Stage
	result, err := r.processPlugin(ctx, logger, deployment, originalDeployment)

	// Update stage if changed
	stage := r.Plugin.ParseStage(deployment)
	if originalStage != stage {
		message := fmt.Sprintf("state transition from %s to %s", originalStage, stage)
		logger.Info(message)
		deployment.Status.Stage = stage
		r.handleStageTransition(ctx, logger, deployment, err)
		r.Recorder.Event(deployment, "Normal", "StageChange", message)
	}

	// Handle cleanup completion
	if common.IsCleanupCompleteStage(deployment.Status.Stage) {
		controllerutil.RemoveFinalizer(deployment, _deploymentCleanedUpFinalizer)
		return ctrl.Result{}, nil
	}

	return result, err
}

// processPlugin processes the deployment using the appropriate plugin
func (r *Reconciler) processPlugin(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment, originalDeployment *v2pb.Deployment) (ctrl.Result, error) {
	defaultResult := ctrl.Result{
		Requeue:      true,
		RequeueAfter: _defaultRequeuePeriod,
	}

	var err error
	// Process based on deployment state following reference pattern
	if common.ShouldCleanup(*deployment) {
		if !common.IsCleanupStage(deployment.Status.Stage) {
			logger.Info("detected that a cleanup should occur")
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		}
		cleanupPlugin := r.Plugin.GetCleanupPlugin()
		err = cleanupPlugin.Execute(ctx, logger, deployment)
		if err != nil {
			logger.Error(err, "Cleanup plugin processing failed")
			return defaultResult, err
		}
	} else if common.RolloutInProgress(*deployment) {
		// Health check gate
		observability := oss.ObservabilityContext{Logger: logger}
		isHealthy, healthErr := r.Plugin.HealthCheckGate(ctx, observability, deployment)
		if healthErr != nil {
			logger.Error(healthErr, "failed to get the health check")
			return defaultResult, healthErr
		}

		desiredModelChanged := common.ShouldRollback(*deployment)
		rollbackAlertsEnabled := common.RollbackAlertsEnabled(*deployment)
		if (!isHealthy || desiredModelChanged) && rollbackAlertsEnabled {
			if !common.IsRollbackStage(deployment.GetStatus().Stage) {
				deployment.Status.Message = fmt.Sprintf("Detected that a rollback should occur due to alert firing=[%v], or due to the desired model changing=[%v]", !isHealthy, desiredModelChanged)
				logger.Info("detected that a rollback should occur")
				deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
			}
			rollbackPlugin := r.Plugin.GetRollbackPlugin()
			err = rollbackPlugin.Execute(ctx, logger, deployment)
			if err != nil {
				logger.Error(err, "Rollback plugin processing failed")
				return defaultResult, err
			}
		} else {
			rolloutPlugin, pluginErr := r.Plugin.GetRolloutPlugin(ctx, deployment)
			if pluginErr != nil {
				logger.Error(pluginErr, "failed to retrieve rollout plugin")
				return defaultResult, pluginErr
			}
			err = rolloutPlugin.Execute(ctx, logger, deployment)
			if err != nil {
				logger.Error(err, "Rollout plugin processing failed")
				return defaultResult, err
			}
		}
	} else if common.TriggerNewRollout(*deployment) {
		logger.Info("detected new rollout")
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision

		if !common.ShouldSkipRollout(*deployment) {
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
			rolloutPlugin, pluginErr := r.Plugin.GetRolloutPlugin(ctx, deployment)
			if pluginErr != nil {
				logger.Error(pluginErr, "failed to retrieve rollout plugin")
				return defaultResult, pluginErr
			}
			err = rolloutPlugin.Execute(ctx, logger, deployment)
			if err != nil {
				logger.Error(err, "Rollout plugin processing failed")
				return defaultResult, err
			}
		}
	} else if common.InSteadyState(*deployment) {
		steadyStatePlugin := r.Plugin.GetSteadyStatePlugin()
		err = steadyStatePlugin.Execute(ctx, logger, deployment)
		if err != nil {
			logger.Error(err, "Steady state plugin processing failed")
			return defaultResult, err
		}
	}

	// Get final state from plugin
	observability := oss.ObservabilityContext{Logger: logger}
	status, getStateErr := r.Plugin.GetState(ctx, observability, deployment)
	if getStateErr != nil {
		logger.Error(getStateErr, "Failed to get deployment state")
		return defaultResult, getStateErr
	}
	deployment.Status = status

	return defaultResult, err
}

// handleStageTransition handles deployment stage transitions following reference pattern
func (r *Reconciler) handleStageTransition(
	ctx context.Context,
	logger logr.Logger,
	deployment *v2pb.Deployment,
	err error) bool {

	var messages []string
	if !common.IsTerminalStage(deployment.Status.Stage) {
		if deployment.Status.Message != "" {
			messages = append(messages, deployment.Status.Message)
		}
		if err != nil {
			messages = append(messages, fmt.Sprintf("Error from latest reconciliation: %+v", err))
		}
		if len(messages) > 0 {
			deployment.Status.Message = fmt.Sprintf("%s", messages[0])
		}
		return false
	}

	// Handle terminal stages
	switch deployment.Status.Stage {
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE:
		// Graduate the candidate revision
		deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		deployment.Status.Message = "Rollout completed successfully"
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED:
		messages = append(messages, "Failed to rollout deployment")
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
	case v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE:
		// Clear revisions
		deployment.Status.CurrentRevision = nil
		deployment.Status.CandidateRevision = nil
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_EMPTY
		deployment.Status.Message = "Cleanup completed successfully"
	case v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		messages = append(messages, "Failed to cleanup deployment")
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
	case v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE:
		deployment.Status.Message = "Rollback completed successfully"
	case v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:
		messages = append(messages, "Failed to rollback deployment")
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
	}

	if err != nil {
		messages = append(messages, fmt.Sprintf("Error from latest reconciliation: %+v", err))
	}

	if len(messages) > 0 {
		logger.Info(fmt.Sprintf("%s", messages[0]))
	}

	return true
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

// SetupWithManager sets up the controller with the Manager following reference pattern
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Log = mgr.GetLogger().WithName(_deploymentKey)
	r.Recorder = mgr.GetEventRecorderFor(_deploymentKey)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Deployment{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: _maximumConcurrentReconciles}).
		Complete(r)
}
