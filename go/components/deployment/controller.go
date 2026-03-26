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

// Package deployment provides Kubernetes controller logic for managing ML model deployments.
//
// The deployment controller implements progressive rollout strategies for machine learning models,
// enabling safe, gradual deployment of new models to production infrastructure.
//
// The controller uses a plugin-based architecture with lifecycle stages:
//
//	Rollout:     Validation → Asset Preparation → Resource Acquisition → Traffic Routing → Completion
//	Rollback:    Triggered on failures or manual intervention
//	Cleanup:     Resource deallocation on deletion
//	Steady State: Ongoing monitoring and health checks
package deployment

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	protoapi "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	_defaultRequeuePeriod  = 10 * time.Second
	_reconciliationTimeout = 60 * time.Second

	_deploymentCleanedUpFinalizer = "deployments.michelangelo.uber.com/finalizer"

	_deploymentRolloutCount = "deployment.rollout.count"

	_deploymentRollbackReason = "deployment.rollback.reason"

	// this is the concurrency reconcile loops for deployment, it can be tuned if needed.
	_maximumConcurrentReconciles = 10

	_alertFiredMessage          = "Alert fired"
	_desiredModelChangedMessage = "Desired model changed"

	_timeFormat = "20060102-121314"
)

// Reconciler reconciles a Deployment object.
//
// All fields are unexported. Exported struct fields become permanent public API surface —
// external packages can depend on them directly, making future removal a breaking change.
// All dependencies are injected via NewReconciler() or SetupWithManager() instead.
// This follows the same pattern used by controllers in kubernetes/kubernetes.
type Reconciler struct {
	api.Handler
	log               logr.Logger
	recorder          record.EventRecorder
	registrar         pluginmanager.Registrar[plugins.Plugin]
	engine            conditionInterfaces.Engine[*v2pb.Deployment]
	revisionManager   revision.Manager
	scope             tally.Scope
	apiHandlerFactory apiHandler.Factory
	auditLogEmitter   logging.AuditLog
}

// NewReconciler returns a new model deployment reconciler.
func NewReconciler(apiHandlerFactory apiHandler.Factory, registrar pluginmanager.Registrar[plugins.Plugin]) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: apiHandlerFactory,
		registrar:         registrar,
		engine:            defaultengine.NewDefaultEngine[*v2pb.Deployment](createEngineLogger()),
		revisionManager:   revision.NewNoOpManager(),
		scope:             tally.NoopScope,
		auditLogEmitter:   &logging.DummyAuditLog{},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = mgr.GetLogger().
		WithName(_deploymentKey)
	r.recorder = mgr.GetEventRecorderFor(_deploymentKey)
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Deployment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
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
	log := r.log.WithValues(_deploymentKey, req.NamespacedName.String())
	ctx, cancel := context.WithTimeout(ctx, _reconciliationTimeout)
	defer cancel()
	defer func() {
		if err := recover(); err != nil {
			log.Error(fmt.Errorf("%+v", err), "panic occurred during deployment reconcile")
		}
	}()

	metrics := NewControllerMetrics(r.scope)
	defer metrics.reconcileMetrics.duration.Start().Stop()
	metrics.reconcileMetrics.count.Inc(1)

	sw := metrics.retrieveResourceMetrics.duration.Start()
	metrics.retrieveResourceMetrics.count.Inc(1)
	var deployment v2pb.Deployment
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
func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, metrics *ControllerMetrics, deployment *v2pb.Deployment, originalDeployment *v2pb.Deployment) (ctrl.Result, error) {
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

	// Check if we've reached max attempts or if condition is satisfied but terminal.
	// For successful terminal conditions, we should continue processing to allow stage progression.
	if result.IsTerminal && !result.AreSatisfied {
		message := "Maximum attempts reached to reconcile the resource. Will not proceed with rollout or rollback " +
			"until the resource is updated again. If in cleanup, we will no longer reconcile."
		log.Info(message)
		r.recorder.Event(deployment, _normalType, _earlyTerminationEvent, message)
		metrics.terminalCounter.Inc(1)
		newStage, shouldRequeue := getTerminalStage(*deployment)
		stage = newStage
		if shouldRequeue {
			result.Result = defaultResult
		}
		runtimeCtx := plugins.RequestContext{
			Deployment: deployment,
			Logger:     log,
		}
		plugin.PopulateDeploymentLogs(ctx, runtimeCtx, deployment)
	} else if result.IsTerminal && result.AreSatisfied {
		// Successful terminal condition - allow progression by ensuring requeue
		result.Result = ctrl.Result{
			Requeue:      true,
			RequeueAfter: _defaultRequeuePeriod,
		}
	}

	log = log.WithValues(_originalStageKey, originalStage).WithValues(_newStageKey, stage)

	if originalStage != stage {
		message := fmt.Sprintf("state transition from %s to %s", originalStage, stage)
		log.Info(message)
		deployment.Status.Stage = stage
		terminal := r.handleStageTransition(ctx, metrics, deployment, err)
		// TODO(#534): Enable these once revision codes are migrated:
		// - Either implement UpsertDeploymentRevision properly with full error handling
		// - Or permanently remove revision management infrastructure if not needed
		// - See issue #534 for discussion
		// upsertErr := UpsertDeploymentRevision(ctx, deployment, r.RevisionManager)
		// if upsertErr != nil {
		//	log.Info(fmt.Sprintf("fail to upsert deployment revision. Proceeding with deployment. Error: %+v", upsertErr))
		// }
		// Make sure that we only set the conditions to nil after the upserting the revision, so we keep track of the
		// latest set of conditions to render.
		if terminal {
			// Guard clause: extracted to handleTerminalTransition() to keep this block
			// at ≤2 levels of nesting. Previously this was 4 levels deep:
			// reconcile → originalStage!=stage → terminal → stage check.
			r.handleTerminalTransition(ctx, log, plugin, deployment)
		}
		r.recorder.Event(deployment, _normalType, _stageChangeEvent, message)
	}

	// TODO(#550): Make the GetState call return just the deployment state instead of the entire status payload
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

	if IsCleanupCompleteStage(deployment.Status.Stage) {
		// If the resource is in cleanup completion stage, then it is eligible for deletion.
		// Since we do not expect this resource to be reconciled (until new user action), the finalizer will not be
		// added again. If there is a new user action, then it is reasonable to avoid deletion. Conversely, if the
		// resource is deleted before any new user action, that new user action will fail.
		controllerutil.RemoveFinalizer(deployment, _deploymentCleanedUpFinalizer)

		// We only want to delete all revisions when the deployment is marked for deletion.
		if !deployment.GetDeletionTimestamp().IsZero() {
			err = r.revisionManager.DeleteAllRevisions(ctx, deployment.GetNamespace(), deployment.GetName(), "Deployment")
			if err != nil {
				log.Error(err, "Failed to delete all revisions for deployment. This is not critical. "+
					"Note that if a revision with the same name is recreated, the deployment history may be inaccurate.")
			}
		}

		return ctrl.Result{}, nil
	}

	return result.Result, err
}

func (r *Reconciler) processPlugin(ctx context.Context, log logr.Logger, metrics *ControllerMetrics, plugin plugins.Plugin, deployment *v2pb.Deployment, originalDeployment *v2pb.Deployment) (conditionInterfaces.Result, error) {
	// This is just the default result.
	result := conditionInterfaces.Result{
		Result: ctrl.Result{
			Requeue:      true,
			RequeueAfter: _defaultRequeuePeriod,
		},
	}

	var err error
	var conditionPlugin conditionInterfaces.Plugin[*v2pb.Deployment]

	// TODO(#551): Add runtime context to match Uber internal pattern exactly
	// The Uber internal code uses: runtimeContext := conditions.NewRequestContext(log, r.Recorder)
	// and passes it to all Engine.Run() calls: r.engine.Run(ctx, runtimeContext, conditionPlugin, deployment)
	// This requires updating the Engine interface to match Uber's 4-parameter signature
	// For now, our simplified Engine interface uses 3 parameters: Engine.Run(ctx, conditionPlugin, deployment)

	if ShouldCleanup(*deployment) {
		if !IsCleanupStage(deployment.Status.Stage) {
			log.Info("detected that a cleanup should occur")
			metrics.cleanupMetrics.initiatedCount.Inc(1)
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		}

		conditionPlugin = plugin.GetCleanupPlugin()
		result, err = r.engine.Run(ctx, conditionPlugin, deployment)
		if err != nil {
			log.Error(err, "Cleanup plugin processing failed with error")
			return result, err
		}
	} else if RolloutInProgress(*deployment) {
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

		desiredModelChanged := ShouldRollback(*deployment)
		rollbackAlertsEnabled := RollbackAlertsEnabled(*deployment)
		if (!isHealthy || desiredModelChanged) && rollbackAlertsEnabled {
			if !IsRollbackStage(deployment.GetStatus().Stage) {
				deployment.Status.Message = fmt.Sprintf("Detected that a rollback should occur due to alert firing=[%v], or due to the desired model changing=[%v]", isHealthy, desiredModelChanged)
				log.Info("detected that a rollback should occur")
				metrics.rollbackMetrics.initiatedCount.Inc(1)
				// This should rollback check only checks if the current model being rolled out doesn't match the target
				// model. In these cases, we need to stop the rollout.
				deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
				r.updateRollbackReason(deployment, isHealthy)
			}

			conditionPlugin = plugin.GetRollbackPlugin()
			result, err = r.engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollback plugin processing failed with error")
				return result, err
			}
		} else {
			conditionPlugin, err = plugin.GetRolloutPlugin(ctx, deployment)
			if err != nil {
				log.Error(err, "failed to retrieve rollout plugin",
					"operation", "get_rollout_plugin",
					"namespace", deployment.Namespace,
					"deployment", deployment.Name)
				return result, err
			}
			result, err = r.engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollout plugin processing failed with error")
				return result, err
			}
		}
	} else if TriggerNewRollout(*deployment) {
		log.Info("detected new rollout")
		metrics.rolloutMetrics.initiatedCount.Inc(1)
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision

		// cleanup rollback reason from previous deployment (if any)
		delete(deployment.Annotations, _deploymentRollbackReason)

		if IsEmergencyRollout(*deployment) {
			// Log emergency rollout for audit purposes
			log.Info("Emergency rollout detected",
				"deployment", fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name),
				"issue_link", deployment.Spec.Strategy.GetBlast().GetIssueLink(),
				"with_rollback_alerts", deployment.Spec.Strategy.GetBlast().GetWithRollbackTrigger())
		}

		if !ShouldSkipRollout(*deployment) {
			r.incrementRolloutCount(deployment, log)
			deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
			conditionPlugin, err = plugin.GetRolloutPlugin(ctx, deployment)
			if err != nil {
				log.Error(err, "failed to retrieve rollout plugin",
					"operation", "get_rollout_plugin",
					"namespace", deployment.Namespace,
					"deployment", deployment.Name)
				return result, err
			}
			result, err = r.engine.Run(ctx, conditionPlugin, deployment)
			if err != nil {
				log.Error(err, "Rollout plugin processing failed with error")
				return result, err
			}
		}
	} else if InSteadyState(*deployment) {
		metrics.steadyStateMetrics.initiatedCount.Inc(1)

		conditionPlugin = plugin.GetSteadyStatePlugin()
		result, err = r.engine.Run(ctx, conditionPlugin, deployment)
		if err != nil {
			log.Error(err, "Steady state plugin processing failed with error")
			return result, err
		}
	}
	removeConditionsForDeployment(deployment, conditionPlugin)
	return result, nil
}

// handleTerminalTransition clears active conditions and populates the deployment message
// after reaching a terminal stage.
//
// Extracted from reconcile() to keep the stage-transition block at ≤2 nesting levels.
// Without this helper, the caller had 4 levels: reconcile → originalStage!=stage → terminal → stage check.
func (r *Reconciler) handleTerminalTransition(ctx context.Context, log logr.Logger, plugin plugins.Plugin, deployment *v2pb.Deployment) {
	// Capture a snapshot of failing conditions before clearing them, so that
	// operators can inspect what went wrong after the fact.
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED {
		deployment.Status.ConditionsSnapshot = deployment.Status.Conditions
	}
	deployment.Status.Conditions = nil

	// Populate the human-readable message for terminal stages that are visible to users.
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE ||
		deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED {
		runtimeCtx := plugins.RequestContext{
			Deployment: deployment,
			Logger:     log,
		}
		plugin.PopulateMessage(ctx, runtimeCtx, deployment)
	}
}

// handleStageTransition will ensure that the deployment controller performs the correct set of actions
// whenever there is a stage transition for the particular deployment resource. It will also return whether
// or not the deployment is terminal.
func (r *Reconciler) handleStageTransition(
	ctx context.Context,
	metrics *ControllerMetrics,
	deployment *v2pb.Deployment,
	err error,
) bool {
	var messages []string

	if !IsTerminalStage(deployment.Status.Stage) {
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

	log := r.log.WithValues(_deploymentKey, fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name))

	switch deployment.Status.Stage {
	// Terminal stages
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE:
		metrics.rolloutMetrics.completedCount.Inc(1)
		// Graduate the candidate revision.
		deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
		metrics.createDeploymentEventMetrics.count.Inc(1)
		createDeploymentEventErr := r.createDeploymentEvent(ctx, deployment)
		if createDeploymentEventErr != nil {
			metrics.createDeploymentEventMetrics.errorCount.Inc(1)
			errMsg := "Failed to create DeploymentEvent object during ROLLOUT_COMPLETE"
			log.Error(createDeploymentEventErr, errMsg)
			messages = append(messages, errMsg)
		}
		break
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED:
		metrics.rolloutMetrics.failedCount.Inc(1)
		messages = append(messages, "Failed to rollout deployment")
		break
	case v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE:
		metrics.cleanupMetrics.completedCount.Inc(1)

		// create DeploymentEvent before clearing CurrentRevision below since it's required to determine if the
		// Deployment is for an LLM
		metrics.createDeploymentEventMetrics.count.Inc(1)
		createDeploymentEventErr := r.createDeploymentEvent(ctx, deployment)
		if createDeploymentEventErr != nil {
			metrics.createDeploymentEventMetrics.errorCount.Inc(1)
			errMsg := "Failed to create DeploymentEvent object during CLEAN_UP_COMPLETE"
			log.Error(createDeploymentEventErr, errMsg)
			messages = append(messages, errMsg)
		}

		// Clear candidate and current revisions.
		deployment.Status.CurrentRevision = nil
		deployment.Status.CandidateRevision = nil
		break
	case v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		metrics.cleanupMetrics.failedCount.Inc(1)
		messages = append(messages, "Failed to cleanup deployment")
		break
	case v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE:
		metrics.rollbackMetrics.completedCount.Inc(1)
		break
	case v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:
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

func (r *Reconciler) getPlugin(deployment v2pb.Deployment) (plugins.Plugin, error) {
	if deployment.Spec.Definition == nil {
		return r.registrar.GetPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", &deployment)
	}

	definition := deployment.Spec.Definition
	return r.registrar.GetPlugin(definition.Type.String(), definition.SubType, &deployment)
}

func (r *Reconciler) incrementRolloutCount(deployment *v2pb.Deployment, log logr.Logger) {
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

func (r *Reconciler) updateRollbackReason(deployment *v2pb.Deployment, isHealthy bool) {
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
	tags := map[string]string{
		_namespaceTag: namespace,
	}
	return plugins.ObservabilityContext{
		Logger: log,
		Scope:  r.scope.Tagged(tags),
	}
}

// getTerminalStage retrieves the stage whenever the plugin has run for too long. It also returns a boolean indicating
// whether a requeue should occur or not.
func getTerminalStage(deployment v2pb.Deployment) (v2pb.DeploymentStage, bool) {
	// It is necessary to reconcile for rollout and rollback at this point because we still need to check the health
	// of the currently deployed revision. It is safe to do so because candidate and current are the same,
	// so a new deployment will not trigger until the candidate is cleared, or the desired revision changes.
	// Furthermore, the rollout will not continue because we've reached a terminal stage.
	//
	// During cleanup, we will terminate because at this point the status is no longer relevant.
	if IsCleanupStage(deployment.Status.Stage) {
		return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED, false
	} else if IsRollbackStage(deployment.Status.Stage) {
		return v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED, true
	} else if RolloutInProgress(deployment) {
		return v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED, true
	}

	return deployment.Status.Stage, false
}

// createDeploymentEvent creates a deployment event for tracking deployment state transitions
// In simplified version, this is a no-op to maintain structure compatibility with Uber internal code
func (r *Reconciler) createDeploymentEvent(ctx context.Context, deployment *v2pb.Deployment) error {
	// TODO(#552): In full implementation, this would:
	// 1. Marshal the deployment object to protobuf.Any
	// 2. Create a DeploymentEvent resource with the marshaled deployment content
	// 3. Save it to the cluster for audit/tracking purposes
	// For now, this is a no-op but maintains the same function signature as Uber internal
	r.log.V(1).Info("createDeploymentEvent called (no-op in simplified version)",
		"deployment", fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name),
		"stage", deployment.Status.Stage)
	return nil
}

// createDeploymentEventName generates a name for deployment events following Uber internal pattern
func createDeploymentEventName(deploymentName string) string {
	return fmt.Sprintf("%s-%s", deploymentName, time.Now().Format(_timeFormat))
}

// Deployment utility functions - moved from common package
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

// TriggerNewRollout determines if a new rollout should be triggered based on the deployment state.
//
// A rollout is triggered when:
//   - The desired revision differs from the candidate revision (indicating a model change)
//   - AND the deployment is in a terminal stage (ROLLOUT_COMPLETE, ROLLBACK_COMPLETE, etc.) or initialization stage
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if a new rollout should be triggered, false otherwise
func TriggerNewRollout(deployment v2pb.Deployment) bool {
	desiredRevision := deployment.Spec.DesiredRevision
	candidateRevision := deployment.Status.CandidateRevision
	desiredCandidateDiffer := !desiredRevisionEqual(desiredRevision, candidateRevision)
	terminalOrInit := IsTerminalStage(deployment.Status.Stage) || isInitializationStage(deployment.Status.Stage)
	result := desiredCandidateDiffer && terminalOrInit

	return result
}

// ShouldRollback determines if the deployment should be rolled back to a previous version.
//
// A rollback is triggered when:
//   - The desired revision differs from the candidate revision (user requested change mid-rollout)
//   - AND the deployment is NOT in a terminal stage (rollout is in progress)
//   - AND the deployment is NOT in initialization stage
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if a rollback should be initiated, false otherwise
func ShouldRollback(deployment v2pb.Deployment) bool {
	desiredRevision := deployment.Spec.DesiredRevision
	candidateRevision := deployment.Status.CandidateRevision
	return desiredRevision != nil &&
		!desiredRevisionEqual(desiredRevision, candidateRevision) &&
		!IsTerminalStage(deployment.Status.Stage) &&
		!isInitializationStage(deployment.Status.Stage)
}

// RolloutInProgress checks if a model rollout is currently in progress.
//
// A rollout is considered in progress when:
//   - The current revision differs from the candidate revision (models don't match)
//   - AND the deployment is NOT in a terminal stage
//   - AND the deployment is NOT in initialization stage
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if a rollout is actively in progress, false otherwise
func RolloutInProgress(deployment v2pb.Deployment) bool {
	currentRevision := deployment.Status.CurrentRevision
	candidateRevision := deployment.Status.CandidateRevision

	revisionsDiffer := !revisionEqual(currentRevision, candidateRevision)
	notTerminal := !IsTerminalStage(deployment.Status.Stage)
	notInitialization := !isInitializationStage(deployment.Status.Stage)

	result := revisionsDiffer && notTerminal && notInitialization

	return result
}

// InSteadyState checks if the deployment is in a steady state phase.
//
// A deployment is in steady state when it has completed a rollout or rollback and is now
// in a stable operational phase. During steady state, the system performs maintenance tasks
// like monitoring, metric collection, and health checks.
//
// Steady state stages include:
//   - DEPLOYMENT_STAGE_ROLLOUT_COMPLETE: Rollout successfully completed
//   - DEPLOYMENT_STAGE_ROLLBACK_COMPLETE: Rollback successfully completed
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if the deployment is in steady state, false otherwise
func InSteadyState(deployment v2pb.Deployment) bool {
	return deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE ||
		deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
}

// ShouldCleanup determines if the deployment resources should be cleaned up.
//
// Cleanup is initiated when:
//   - The deployment is marked for deletion (deletion timestamp is set)
//   - OR the deployment's deletion spec indicates it should be deleted
//   - OR the desired revision is nil but there are still active current/candidate revisions
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if cleanup should be initiated, false otherwise
func ShouldCleanup(deployment v2pb.Deployment) bool {
	currentRevision := deployment.Status.GetCurrentRevision()
	candidateRevision := deployment.Status.GetCandidateRevision()
	markedForDeletion := !deployment.ObjectMeta.DeletionTimestamp.IsZero()
	return markedForDeletion ||
		deployment.Spec.GetDeletionSpec().GetDeleted() ||
		(deployment.Spec.DesiredRevision == nil &&
			(currentRevision != nil || candidateRevision != nil))
}

// IsTerminalStage checks if the given deployment stage is terminal.
//
// Terminal stages represent stable states where the deployment has reached a final outcome:
//   - Rollout completed successfully
//   - Rollback completed successfully
//   - Cleanup completed
//   - Failed states
//
// Parameters:
//   - stage: The deployment stage to check
//
// Returns:
//   - true if the stage is terminal (no further automatic transitions), false otherwise
func IsTerminalStage(stage v2pb.DeploymentStage) bool {
	_, ok := _terminalStages[stage]
	return ok
}

// IsRollbackStage checks if the deployment is currently in a rollback stage.
//
// Parameters:
//   - stage: The deployment stage to check
//
// Returns:
//   - true if the stage is a rollback stage, false otherwise
func IsRollbackStage(stage v2pb.DeploymentStage) bool {
	_, ok := _rollbackStages[stage]
	return ok
}

// IsCleanupStage checks if the deployment is in a cleanup stage.
//
// Parameters:
//   - stage: The deployment stage to check
//
// Returns:
//   - true if the stage is a cleanup stage, false otherwise
func IsCleanupStage(stage v2pb.DeploymentStage) bool {
	_, ok := _cleanUpStages[stage]
	return ok
}

// IsCleanupCompleteStage checks if the deployment has completed cleanup operations.
//
// Parameters:
//   - stage: The deployment stage to check
//
// Returns:
//   - true if cleanup is complete, false otherwise
func IsCleanupCompleteStage(stage v2pb.DeploymentStage) bool {
	_, ok := _cleanUpCompleteStages[stage]
	return ok
}

func isInitializationStage(stage v2pb.DeploymentStage) bool {
	return stage == v2pb.DEPLOYMENT_STAGE_INVALID
}

// ShouldSkipRollout checks if the rollout can be skipped because the target is already deployed.
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if the rollout can be skipped, false if it should proceed
func ShouldSkipRollout(deployment v2pb.Deployment) bool {
	candidateRevision := deployment.Status.GetCandidateRevision()
	currentRevision := deployment.Status.GetCurrentRevision()
	return candidateRevision != nil && revisionEqual(candidateRevision, currentRevision)
}

// IsEmergencyRollout checks if the deployment is configured for emergency/blast rollout.
//
// Emergency rollouts bypass normal progressive rollout safeguards for rapid deployment:
//   - Immediate 100% traffic switch (no gradual ramp-up)
//   - Reduced validation steps
//   - Used for critical hotfixes or security patches
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if this is an emergency/blast deployment, false for normal progressive rollout
func IsEmergencyRollout(deployment v2pb.Deployment) bool {
	if strategy := deployment.Spec.GetStrategy(); strategy != nil {
		isEmergency := strategy.GetBlast()
		return isEmergency != nil
	}
	return false
}

// RollbackAlertsEnabled checks if automatic rollback based on alerts is enabled.
//
// Parameters:
//   - deployment: The deployment resource to evaluate
//
// Returns:
//   - true if alert-based rollback is enabled, false otherwise
func RollbackAlertsEnabled(deployment v2pb.Deployment) bool {
	if IsEmergencyRollout(deployment) {
		withRollbackAlerts := deployment.Spec.Strategy.GetBlast().GetWithRollbackTrigger()
		return withRollbackAlerts
	}
	return true
}

// Helper functions for revision equality since protobuf doesn't have Equal method
func revisionEqual(a, b *protoapi.ResourceIdentifier) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name && a.Namespace == b.Namespace
}

func desiredRevisionEqual(a, b *protoapi.ResourceIdentifier) bool {
	return revisionEqual(a, b)
}

// createEngineLogger creates a proper zap logger for the engine
func createEngineLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := config.Build()
	if err != nil {
		// Fallback to a basic logger if building fails
		return zap.NewExample()
	}
	return logger
}
