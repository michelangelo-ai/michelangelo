/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"github.com/michelangelo-ai/michelangelo/go/api"
	protoapi "github.com/michelangelo-ai/michelangelo/proto/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/revision"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	_defaultRequeuePeriod  = 20 * time.Minute
	_reconciliationTimeout = 60 * time.Second

	_deploymentCleanedUpFinalizer = "deployments.michelangelo.uber.com/finalizer"

	_deploymentRolloutCount = "deployment.rollout.count"

	_deploymentRollbackReason = "deployment.rollback.reason"

	// this is the concurrency reconcile loops for deployment, it can be tuned if needed.
	_maximumConcurrentReconciles = 10
	_timeFormat                  = "20060102-121314"

	_alertFiredMessage          = "Alert fired"
	_desiredModelChangedMessage = "Desired model changed"
)

// Reconciler reconciles a Deployment object
type Reconciler struct {
	api.Handler
	// TODO: refactor so these are not exported
	Log             logr.Logger
	Recorder        record.EventRecorder
	Registrar       pluginmanager.Registrar[plugins.Plugin]
	Engine          conditionInterfaces.Engine[*types.Deployment]
	RevisionManager revision.Manager
	Scope           interface{}
	apiHandlerFactory apiHandler.Factory
}

// NewReconciler returns a new model deployment reconciler.
func NewReconciler(apiHandlerFactory apiHandler.Factory) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: apiHandlerFactory,
		Registrar:         pluginmanager.NewSimpleRegistrar[plugins.Plugin](logr.Discard()),
		Engine:            defaultengine.NewDefaultEngine[*types.Deployment](zap.NewNop()),
		RevisionManager:   revision.NewNoOpManager(),
		Scope:             NewNoOpScope(),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Log = mgr.GetLogger().
		WithName(_deploymentKey)
	r.Recorder = mgr.GetEventRecorderFor(_deploymentKey)
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler

	// Register the default no-op plugin
	noOpPlugin := plugins.NewNoOpPlugin()
	r.Registrar.RegisterPlugin(types.TARGET_TYPE_INFERENCE_SERVER.String(), "", noOpPlugin)
	r.Registrar.RegisterPlugin(types.TARGET_TYPE_OFFLINE.String(), "", noOpPlugin)
	r.Registrar.RegisterPlugin(types.TARGET_TYPE_MOBILE.String(), "", noOpPlugin)

	return ctrl.NewControllerManagedBy(mgr).
		For(&types.Deployment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{MaxConcurrentReconciles: _maximumConcurrentReconciles}).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the resource closer to the desired state.
//
// This `Reconcile` method differs from `reconcile` in that it does not do anything to move the deployment
// through the various steps required to perform rollout, rollback or cleanup. Its main role is to set up the logger
// with common tags, and save the deployment resource in case any changes are detected.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues(_deploymentKey, req.NamespacedName.String())
	ctx, cancel := context.WithTimeout(ctx, _reconciliationTimeout)
	defer cancel()
	defer func() {
		if err := recover(); err != nil {
			log.Error(fmt.Errorf("%+v", err), "panic occurred during deployment reconcile")
		}
	}()

	metrics := NewControllerMetrics(r.Scope)
	defer metrics.reconcileMetrics.duration.Start().Stop()
	metrics.reconcileMetrics.count.Inc(1)

	sw := metrics.retrieveResourceMetrics.duration.Start()
	metrics.retrieveResourceMetrics.count.Inc(1)
	var deployment types.Deployment
	if err := r.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name,
		&metav1.GetOptions{}, &deployment); err != nil {
		metrics.retrieveResourceMetrics.errorCount.Inc(1)
		if utils.IsNotFoundError(err) {
			log.Error(err, "request made for model deployment that is not found. Ignoring this request")
			return ctrl.Result{}, nil
		}

		log.Error(err, "failed to retrieve model deployment object")
		return ctrl.Result{}, err
	}
	sw.Stop()

	log = log.WithValues(_targetLoggingKey, deployment.Spec.GetDefinition().GetType())
	log = log.WithValues(_desiredModelKey, deployment.Spec.GetDesiredRevision().GetName())
	log = log.WithValues(_candidateModelKey, deployment.Status.GetCandidateRevision().GetName())
	log = log.WithValues(_currentModelKey, deployment.Status.GetCurrentRevision().GetName())

	// Copy by value, not reference, so originalDeployment will never change, even after downstream components change.
	originalDeployment := deployment.DeepCopy()
	result, err := r.reconcile(ctx, log, metrics, &deployment, originalDeployment)
	if err != nil {
		metrics.reconcileMetrics.errorCount.Inc(1)
		log.Error(err, fmt.Sprintf("failed to process deployment"))
		return result, err
	}

	// Update the model deployment resource only if modifications to the object has been made.
	if !reflect.DeepEqual(originalDeployment, &deployment) {
		sw = metrics.updateResourceMetrics.duration.Start()
		metrics.updateResourceMetrics.count.Inc(1)
		// We copy the deployment at this point because the r.Client.Update call below will set the Status object
		// to an empty struct.
		deploymentCopy := deployment.DeepCopy()
		if updateErr := r.Update(ctx, &deployment, &metav1.UpdateOptions{}); updateErr != nil {
			log.Error(updateErr, "Failed to update the deployment resource")
			// We must retry if update fails so return the error.
			return result, err
		}

		// persist the new status.
		deployment.Status = deploymentCopy.Status
		// Do not re-use err here, because it's the state machine failure that we want to be returning.
		if updateErr := r.UpdateStatus(ctx, &deployment, &metav1.UpdateOptions{}); updateErr != nil {
			log.Error(updateErr, "Failed to update the deployment status sub resource")
			// We must retry if update status fails so return the error.
			return result, err
		}
		sw.Stop()
	}

	// Even if there is an error, return nil because it is the plugin's responsibility
	// to determine the retry period. If an error is returned instead, it will requeue immediately.
	return result, nil
}

// reconcile is responsible for all the requirements for reconciling a deployment other than processing the plugin.
// These responsibilities include:
// 1. Retrieving the plugin
// 2. Processing early termination if a plugin continuously fails
// 3. Processing stage transition
// 4. Getting the final state
// 5. Stops reconciliation if cleanup is complete
// 6. Set up the finalizer if it doesn't exist
func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, metrics *ControllerMetrics, deployment *types.Deployment, originalDeployment *types.Deployment) (ctrl.Result, error) {
	defaultResult := ctrl.Result{
		Requeue:      true,
		RequeueAfter: _defaultRequeuePeriod,
	}

	if deployment.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(deployment, _deploymentCleanedUpFinalizer) {
			controllerutil.AddFinalizer(deployment, _deploymentCleanedUpFinalizer)
			if err := r.Update(ctx, deployment, &metav1.UpdateOptions{}); err != nil {
				return defaultResult, fmt.Errorf("failed to add deployment finalizer: %w", err)
			}
		}
	}

	plugin, err := r.getPlugin(*deployment)
	if err != nil {
		log.Error(err, "failed to get deployment plugin")
		return defaultResult, fmt.Errorf("failed to get deployment plugin: %w", err)
	}

	originalStage := deployment.Status.Stage
	result, err := r.processPlugin(ctx, log, metrics, plugin, deployment, originalDeployment)

	// Inject the provider status as a log tag after processing has occurred.
	log = log.WithValues(_providerStatus, deployment.Status.ProviderStatus)
	stage := plugin.ParseStage(deployment)

	// Check if we've reached max attempts. If it is, we should change the deployment status stage
	if result.IsTerminal {
		message := "Maximum attempts reached to reconcile the resource. Will not proceed with rollout or rollback " +
			"until the resource is updated again. If in cleanup, we will no longer reconcile."
		log.Info(message)
		r.Recorder.Event(deployment, _normalType, _earlyTerminationEvent, message)
		metrics.terminalCounter.Inc(1)
		newStage, shouldRequeue := getTerminalStage(*deployment)
		stage = newStage
		if shouldRequeue {
			result.Result = defaultResult
		}
		plugin.PopulateDeploymentLogs(ctx, deployment)
	}

	log = log.WithValues(_originalStageKey, originalStage).WithValues(_newStageKey, stage)

	if originalStage != stage {
		message := fmt.Sprintf("state transition from %s to %s", originalStage, stage)
		log.Info(message)
		deployment.Status.Stage = stage
		terminal := r.handleStageTransition(ctx, metrics, deployment, err)
		// Always attempt to update the deployment revision whenever stage changes.
		upsertErr := UpsertDeploymentRevision(ctx, deployment, r.RevisionManager)
		if upsertErr != nil {
			log.Info(fmt.Sprintf("fail to upsert deployment revision. Proceeding with deployment. Error: %+v", upsertErr))
		}
		// Make sure that we only set the conditions to nil after the upserting the revision, so we keep track of the
		// latest set of conditions to render.
		if terminal {
			// if the rollout failed, we want to render a snapshot of the failing conditions
			if deployment.Status.Stage == types.DEPLOYMENT_STAGE_ROLLOUT_FAILED {
				deployment.Status.ConditionsSnapshot = deployment.Status.Conditions
			}
			deployment.Status.Conditions = nil
			if deployment.Status.Stage == types.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE || deployment.Status.Stage == types.DEPLOYMENT_STAGE_ROLLOUT_FAILED {
				plugin.PopulateMessage(ctx, deployment)
			}
		}
		r.Recorder.Event(deployment, _normalType, _stageChangeEvent, message)
	}

	// TODO: Make the GetState call return just the deployment state instead of the entire status payload
	sw := metrics.getStateMetrics.duration.Start()
	metrics.getStateMetrics.count.Inc(1)
	observability := r.getObservability(log, deployment.Namespace)
	status, getStateErr := plugin.GetState(ctx, observability, deployment)
	if getStateErr != nil {
		metrics.getStateMetrics.errorCount.Inc(1)
		log.Error(getStateErr, "Failed to execute monitoring step. The state may not be up-to-date.")

		return defaultResult, getStateErr
	}
	sw.Stop()
	deployment.Status = status

	if common.IsCleanupCompleteStage(deployment.Status.Stage) {
		// If the resource is in cleanup completion stage, then it is eligible for deletion.
		// Since we do not expect this resource to be reconciled (until new user action), the finalizer will not be
		// added again. If there is a new user action, then it is reasonable to avoid deletion. Conversely, if the
		// resource is deleted before any new user action, that new user action will fail.
		controllerutil.RemoveFinalizer(deployment, _deploymentCleanedUpFinalizer)

		// We only want to delete all revisions when the deployment is marked for deletion.
		if !deployment.GetDeletionTimestamp().IsZero() {
			err = r.RevisionManager.DeleteAllRevisions(ctx, deployment.GetNamespace(), deployment.GetName(), "Deployment")
			if err != nil {
				log.Error(err, "Failed to delete all revisions for deployment. This is not critical. "+
					"Note that if a revision with the same name is recreated, the deployment history may be inaccurate.")
			}
		}

		return ctrl.Result{}, nil
	}

	return result.Result, err
}

func (r *Reconciler) processPlugin(ctx context.Context, log logr.Logger, metrics *ControllerMetrics, plugin plugins.Plugin, deployment *types.Deployment, originalDeployment *types.Deployment) (conditionInterfaces.Result, error) {
	// This is just the default result.
	result := conditionInterfaces.Result{
		Result: ctrl.Result{
			Requeue:      true,
			RequeueAfter: _defaultRequeuePeriod,
		},
	}

	var err error
	var conditionPlugin conditionInterfaces.Plugin[*types.Deployment]
	if common.ShouldCleanup(*deployment) {
		if !common.IsCleanupStage(deployment.Status.Stage) {
			log.Info("detected that a cleanup should occur")
			metrics.cleanupMetrics.initiatedCount.Inc(1)
			deployment.Status.Stage = types.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		}

		conditionPlugin = plugin.GetCleanupPlugin()
		result, err = r.Engine.Run(ctx, conditionPlugin, deployment)
		if err != nil {
			log.Error(err, "Cleanup plugin processing failed with error")
			return result, err
		}
	} else if common.RolloutInProgress(*deployment) {
		sw := metrics.healthCheckGateMetrics.duration.Start()
		metrics.healthCheckGateMetrics.count.Inc(1)
		observability := r.getObservability(log, deployment.Namespace)
		isHealthy, healthGateError := plugin.HealthCheckGate(ctx, observability, deployment)
		if healthGateError != nil {
			metrics.healthCheckGateMetrics.errorCount.Inc(1)
			log.Error(healthGateError, "failed to get the health check ")
			return result, healthGateError
		}
		sw.Stop()

		desiredModelChanged := common.ShouldRollback(*deployment)
		rollbackAlertsEnabled := common.RollbackAlertsEnabled(*deployment)
		if (!isHealthy || desiredModelChanged) && rollbackAlertsEnabled {
			if !common.IsRollbackStage(deployment.Status.Stage) {
				deployment.Status.Message = fmt.Sprintf("Detected that a rollback should occur due to alert firing=[%v], or due to the desired model changing=[%v]", !isHealthy, desiredModelChanged)
				log.Info("detected that a rollback should occur")
				metrics.rollbackMetrics.initiatedCount.Inc(1)
				// This should rollback check only checks if the current model being rolled out doesn't match the target
				// model. In these cases, we need to stop the rollout.
				deployment.Status.Stage = types.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
				r.updateRollbackReason(deployment, isHealthy)
			}

			conditionPlugin = plugin.GetRollbackPlugin()
			result, err = r.Engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollback plugin processing failed with error")
				return result, err
			}
		} else {
			conditionPlugin, err = plugin.GetRolloutPlugin(ctx, deployment)
			if err != nil {
				log.Info("failed to retrieve rollout plugin")
				return result, err
			}
			result, err = r.Engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollout plugin processing failed with error")
				return result, err
			}
		}
	} else if common.TriggerNewRollout(*deployment) {
		log.Info("detected new rollout")
		metrics.rolloutMetrics.initiatedCount.Inc(1)
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision

		//cleanup rollback reason from previous deployment (if any)
		delete(deployment.Annotations, _deploymentRollbackReason)

		if common.IsEmergencyRollout(*deployment) {
			// In simplified version, we just log this
			log.Info("Emergency rollout detected", "jiraLink", deployment.Spec.Strategy.GetBlast().GetJiraLink())
		}

		if !common.ShouldSkipRollout(*deployment) {
			r.incrementRolloutCount(deployment, log)
			deployment.Status.Stage = types.DEPLOYMENT_STAGE_VALIDATION
			conditionPlugin, err = plugin.GetRolloutPlugin(ctx, deployment)
			if err != nil {
				log.Info("failed to retrieve rollout plugin")
				return result, err
			}
			result, err = r.Engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollout plugin processing failed with error")
				return result, err
			}
		}
	} else if common.InSteadyState(*deployment) {
		metrics.steadyStateMetrics.initiatedCount.Inc(1)

		conditionPlugin = plugin.GetSteadyStatePlugin()
		result, err = r.Engine.Run(ctx, conditionPlugin, deployment)
		if err != nil {
			log.Error(err, "Steady state plugin processing failed with error")
			return result, err
		}
	}
	removeConditionsForDeployment(deployment, conditionPlugin)
	return result, nil
}

// handleStageTransition will ensure that the deployment controller performs the correct set of actions
// whenever there is a stage transition for the particular deployment resource. It will also return whether
// or not the deployment is terminal.
func (r *Reconciler) handleStageTransition(
	ctx context.Context,
	metrics *ControllerMetrics,
	deployment *types.Deployment,
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
			deployment.Status.Message = strings.Join(messages, ". ")
		}
		return false
	}

	log := r.Log.WithValues(_deploymentKey, fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name))

	switch deployment.Status.Stage {
	// Terminal stages
	case types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE:
		metrics.rolloutMetrics.completedCount.Inc(1)
		// Graduate the candidate revision.
		deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
		// In simplified version, we skip creating deployment events
		break
	case types.DEPLOYMENT_STAGE_ROLLOUT_FAILED:
		metrics.rolloutMetrics.failedCount.Inc(1)
		messages = append(messages, "Failed to rollout deployment")
		break
	case types.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE:
		metrics.cleanupMetrics.completedCount.Inc(1)
		// Clear candidate and current revisions.
		deployment.Status.CurrentRevision = nil
		deployment.Status.CandidateRevision = nil
		break
	case types.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		metrics.cleanupMetrics.failedCount.Inc(1)
		messages = append(messages, "Failed to cleanup deployment")
		break
	case types.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE:
		metrics.rollbackMetrics.completedCount.Inc(1)
		break
	case types.DEPLOYMENT_STAGE_ROLLBACK_FAILED:
		metrics.rollbackMetrics.failedCount.Inc(1)
		messages = append(messages, "Failed to rollback deployment")
		break
	default:
	}

	// Only log conditional message when the deployment stage is terminal, and only log the first actor that is not
	// true. Otherwise, the message will have too many entries and be impossible to read.
	for _, condition := range deployment.Status.GetConditions() {
		if condition.Status != protoapi.CONDITION_STATUS_TRUE {
			messages = append(messages, fmt.Sprintf("Actor: %s, Message: %s, Reason: %s, UpdatedTimestamp: %d", condition.Type, condition.Message, condition.Reason, condition.LastUpdatedTimestamp))
			continue
		}
	}

	if err != nil {
		messages = append(messages, fmt.Sprintf("Error from latest reconciliation: %+v", err))
	}

	if len(messages) > 0 {
		log.Info(strings.Join(messages, ". "))
	} else {
		deployment.Status.Message = ""
	}

	return true
}

func (r *Reconciler) getPlugin(deployment types.Deployment) (plugins.Plugin, error) {
	if deployment.Spec.Definition == nil {
		return r.Registrar.GetPlugin(types.TARGET_TYPE_INFERENCE_SERVER.String(), "", &deployment)
	}

	definition := deployment.Spec.Definition
	return r.Registrar.GetPlugin(definition.Type.String(), definition.SubType, &deployment)
}

func (r *Reconciler) incrementRolloutCount(deployment *types.Deployment, log logr.Logger) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	countStr, ok := deployment.Annotations[_deploymentRolloutCount]
	if !ok {
		deployment.Annotations[_deploymentRolloutCount] = "0"
	} else {
		count, err := strconv.Atoi(countStr)
		if err != nil {
			log.Error(err, "failed to parse rollout count")
			deployment.Annotations[_deploymentRolloutCount] = "0"
			return
		}
		newCount := strconv.Itoa(count + 1)
		deployment.Annotations[_deploymentRolloutCount] = newCount
	}
}

func (r *Reconciler) updateRollbackReason(deployment *types.Deployment, isHealthy bool) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}

	if !isHealthy {
		deployment.Annotations[_deploymentRollbackReason] = _alertFiredMessage
	} else {
		deployment.Annotations[_deploymentRollbackReason] = _desiredModelChangedMessage
	}
}

func (r *Reconciler) getObservability(log logr.Logger, namespace string) plugins.ObservabilityContext {
	return plugins.ObservabilityContext{
		Logger: log,
		Scope:  r.Scope,
	}
}

// getTerminalStage retrieves the stage whenever the plugin has run for too long. It also returns a boolean indicating
// whether a requeue should occur or not.
func getTerminalStage(deployment types.Deployment) (types.DeploymentStage, bool) {
	// It is necessary to reconcile for rollout and rollback at this point because we still need to check the health
	// of the currently deployed revision. It is safe to do so because candidate and current are the same,
	// so a new deployment will not trigger until the candidate is cleared, or the desired revision changes.
	// Furthermore, the rollout will not continue because we've reached a terminal stage.
	//
	// During cleanup, we will terminate because at this point the status is no longer relevant.
	if common.IsCleanupStage(deployment.Status.Stage) {
		return types.DEPLOYMENT_STAGE_CLEAN_UP_FAILED, false
	} else if common.IsRollbackStage(deployment.Status.Stage) {
		return types.DEPLOYMENT_STAGE_ROLLBACK_FAILED, true
	} else if common.RolloutInProgress(deployment) {
		return types.DEPLOYMENT_STAGE_ROLLOUT_FAILED, true
	}

	return deployment.Status.Stage, false
}