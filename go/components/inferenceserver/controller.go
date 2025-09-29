package inferenceserver

import (
	"context"
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	_inferenceServerCleanedUpFinalizer = "inferenceservers.michelangelo.api/finalizer"
)

// Reconciler reconciles InferenceServer objects
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Gateway  gateways.Gateway
	Plugins  plugins.PluginRegistry
}

// Reconcile handles InferenceServer CRD reconciliation
// Following production patterns: timeout management, structured logging, change detection
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Setup structured logging with trace context
	logger := ctrl.LoggerFrom(ctx).WithName(ControllerName).WithValues(
		"inferenceserver", req.NamespacedName,
		"reconcileTime", time.Now().Format(time.RFC3339),
	)

	// Set timeout for reconciliation - following production pattern
	reconcileCtx, cancel := context.WithTimeout(ctx, ReconcilerTimeout)
	defer cancel()

	// Use internal reconcile method with proper error handling
	result, err := r.reconcile(reconcileCtx, req.NamespacedName, logger)

	// Production pattern: never return errors, use events instead
	if err != nil {
		logger.Error(err, "Reconciliation failed")
		// Record event for user visibility - create minimal object for event
		eventObj := &v2pb.InferenceServer{}
		eventObj.Name = req.Name
		eventObj.Namespace = req.Namespace
		r.Recorder.Event(eventObj, corev1.EventTypeWarning, "ReconciliationError", err.Error())
		// Return success to avoid exponential backoff
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}

	return result, nil
}

// reconcile is the internal reconciliation logic following Uber production pattern
func (r *Reconciler) reconcile(ctx context.Context, namespacedName client.ObjectKey, logger logr.Logger) (ctrl.Result, error) {
	// Fetch the InferenceServer instance
	var inferenceServer v2pb.InferenceServer
	if err := r.Get(ctx, namespacedName, &inferenceServer); err != nil {
		if errors.IsNotFound(err) {
			// Clean up orphaned resources following Uber pattern
			logger.Info("InferenceServer resource not found, cleaning up any orphaned resources")
			orphanedServer := v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
				},
			}
			_, cleanupErr := r.handleDeletion(ctx, logger, &orphanedServer)
			if cleanupErr != nil {
				logger.Error(cleanupErr, "Failed to clean up orphaned resources")
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Deep copy for change detection (Uber production pattern)
	originalInferenceServer := inferenceServer.DeepCopy()

	// Update external details first (like Uber's plugin.UpdateDetails)
	err := r.updateExternalDetails(ctx, logger, &inferenceServer)
	if err != nil {
		logger.Error(err, "Failed to update external details, proceeding with reconciliation")
	}

	// Determine plugin based on deletion state
	var plugin plugins.Plugin
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		backendPlugin, pluginErr := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if pluginErr != nil {
			logger.Error(pluginErr, "Failed to get backend plugin")
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
		plugin = backendPlugin.GetDeletionPlugin(&inferenceServer)
	} else {
		backendPlugin, pluginErr := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if pluginErr != nil {
			logger.Error(pluginErr, "Failed to get backend plugin")
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
		plugin = backendPlugin.GetCreationPlugin()
	}

	// Run the plugin engine (like Uber's engine.Run)
	engine := plugins.NewEngine()
	conditionResult, err := engine.Run(ctx, logger, plugin, &inferenceServer)
	if err != nil {
		logger.Error(err, "Plugin processing failed")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil // Never return errors, use events instead (Uber pattern)
	}

	// Parse state from conditions (like Uber's plugin.ParseState)
	state := parseStateFromConditions(&inferenceServer)
	inferenceServer.Status.State = state

	// Only update if there are changes (Uber production pattern)
	if !reflect.DeepEqual(originalInferenceServer, &inferenceServer) {
		logger.Info("Updating inference server state",
			"oldState", originalInferenceServer.Status.State,
			"newState", inferenceServer.Status.State)

		// Update observed generation and timestamp
		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)

		if err := r.Status().Update(ctx, &inferenceServer); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil // Never return errors (Uber pattern)
		}
	}

	// Convert condition result to appropriate requeue strategy
	if conditionResult != nil && conditionResult.Status == apipb.CONDITION_STATUS_FALSE {
		// Continue active monitoring if there are failing conditions
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}

	// Use steady state requeue if everything is healthy
	return ctrl.Result{RequeueAfter: SteadyStateRequeueAfter}, nil
}

// updateExternalDetails fetches current state from infrastructure (like production plugin.UpdateDetails)
func (r *Reconciler) updateExternalDetails(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	// Skip if resource is being deleted
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Skip if we haven't attempted creation yet (still in initial CREATING state)
	if inferenceServer.Status.ObservedGeneration == 0 || inferenceServer.Status.State == StateCreating {
		return nil
	}

	// Get current status from gateway
	statusResp, err := r.Gateway.GetInfrastructureStatus(ctx, logger, gateways.InfrastructureStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
	if err != nil {
		// Don't fail reconciliation for status check errors
		logger.V(1).Info("Failed to get infrastructure status", "error", err)
		return nil
	}

	// Update status based on external state
	if statusResp.State != inferenceServer.Status.State {
		logger.Info("External state change detected",
			"currentState", inferenceServer.Status.State,
			"externalState", statusResp.State)

		inferenceServer.Status.State = statusResp.State
		inferenceServer.Status.ProviderMetadata = statusResp.Message

		// Record state transition events
		switch statusResp.State {
		case StateServing:
			r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, "CreationCompleted", "InferenceServer creation completed successfully")
		case StateFailed:
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, "CreationFailed", "InferenceServer creation failed")
		}
	}

	return nil
}

// handleCreation manages the creation and update lifecycle
func (r *Reconciler) handleCreation(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	logger.Info("Handling InferenceServer creation/update")

	// Add finalizer if not present (required for proper cleanup)
	if !controllerutil.ContainsFinalizer(inferenceServer, _inferenceServerCleanedUpFinalizer) {
		logger.Info("Adding finalizer for proper deletion handling")
		controllerutil.AddFinalizer(inferenceServer, _inferenceServerCleanedUpFinalizer)
		if err := r.Update(ctx, inferenceServer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		// Return early to let the finalizer update happen
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Initialize status if needed (first time setup)
	if inferenceServer.Status.ObservedGeneration == 0 {
		logger.Info("Initializing InferenceServer status")

		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.State = StateCreating
		inferenceServer.Status.Conditions = []*apipb.Condition{}
		inferenceServer.Status.CreateTime = time.Now().Format(time.RFC3339)
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)

		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, "CreationStarted", "InferenceServer creation started")

		// Return early to let status update happen in main reconcile loop
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Check if generation has changed (spec update)
	if inferenceServer.Status.ObservedGeneration < inferenceServer.Generation {
		logger.Info("Spec generation changed, updating",
			"oldGeneration", inferenceServer.Status.ObservedGeneration,
			"newGeneration", inferenceServer.Generation)

		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		// Don't change state here, let the infrastructure update handle it
	}

	// Only create infrastructure if we're in creating state
	if inferenceServer.Status.State == StateCreating {
		logger.Info("Creating infrastructure using plugin system")

		// Get the appropriate plugin for this backend type
		plugin, err := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if err != nil {
			logger.Error(err, "Failed to get plugin for backend type", "backendType", inferenceServer.Spec.BackendType)
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, "CreationFailed", fmt.Sprintf("No plugin available: %v", err))

			inferenceServer.Status.State = StateFailed
			inferenceServer.Status.ProviderMetadata = fmt.Sprintf("Plugin not found: %v", err)

			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}

		// Execute creation plugin
		creationPlugin := plugin.GetCreationPlugin()
		engine := plugins.NewEngine()

		_, err = engine.Run(ctx, logger, creationPlugin, inferenceServer)
		if err != nil {
			logger.Error(err, "Failed to execute creation actors")
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, "CreationFailed", fmt.Sprintf("InferenceServer creation failed: %v", err))

			inferenceServer.Status.State = StateFailed
			inferenceServer.Status.ProviderMetadata = fmt.Sprintf("Creation failed: %v", err)

			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
	}

	// Configure proxy when server becomes ready (only do this once)
	if inferenceServer.Status.State == StateServing {
		// Check if proxy is already configured
		if inferenceServer.Status.ProviderMetadata != "proxy-configured" {
			logger.Info("Configuring proxy for serving server")

			proxyErr := r.Gateway.ConfigureProxy(ctx, logger, gateways.ProxyConfigRequest{
				InferenceServer: inferenceServer.Name,
				Namespace:       inferenceServer.Namespace,
				ModelName:       inferenceServer.Name, // Use server name as model name for now
				BackendType:     inferenceServer.Spec.BackendType,
			})
			if proxyErr != nil {
				logger.Error(proxyErr, "Failed to configure proxy")
				// Don't fail the reconciliation for proxy errors
			} else {
				inferenceServer.Status.ProviderMetadata = "proxy-configured"
			}
		}
	}

	// Determine requeue strategy based on current state
	switch inferenceServer.Status.State {
	case StateCreating:
		logger.V(1).Info("InferenceServer still creating, requeuing")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	case StateServing:
		logger.V(1).Info("InferenceServer serving, steady state requeue")
		return ctrl.Result{RequeueAfter: SteadyStateRequeueAfter}, nil
	case StateFailed:
		logger.Info("InferenceServer failed, requeuing for retry")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	default:
		logger.Info("Unknown state, requeuing", "state", inferenceServer.Status.State)
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}
}

// handleDeletion manages the deletion lifecycle
func (r *Reconciler) handleDeletion(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	logger.Info("Handling InferenceServer deletion")

	// Delete infrastructure using plugin system
	plugin, err := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
	if err != nil {
		logger.Error(err, "Failed to get plugin for deletion", "backendType", inferenceServer.Spec.BackendType)
		// If plugin not found, consider deletion complete
		logger.Info("Plugin not found, considering deletion complete")
		inferenceServer.Status.State = StateDeleted
		return ctrl.Result{}, nil
	}

	// Execute deletion actors
	deletionPlugin := plugin.GetDeletionPlugin(inferenceServer)
	engine := plugins.NewEngine()

	_, err = engine.Run(ctx, logger, deletionPlugin, inferenceServer)
	if err != nil {
		logger.Error(err, "Failed to execute deletion actors")
		r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, "DeletionFailed", fmt.Sprintf("InferenceServer deletion failed: %v", err))
		// Production pattern: don't return error, continue with requeue
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// parseStateFromConditions parses the overall state from individual conditions
// This follows Uber's pattern of deriving resource state from condition status
func parseStateFromConditions(inferenceServer *v2pb.InferenceServer) v2pb.InferenceServerState {
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		// Resource is being deleted
		return StateDeleting
	}

	if len(inferenceServer.Status.Conditions) == 0 {
		// No conditions yet, starting creation
		return StateCreating
	}

	// Check if all conditions are healthy
	allHealthy := true
	hasFailure := false

	for _, condition := range inferenceServer.Status.Conditions {
		if condition == nil {
			continue
		}
		switch condition.Status {
		case apipb.CONDITION_STATUS_FALSE:
			hasFailure = true
			allHealthy = false
		case apipb.CONDITION_STATUS_UNKNOWN:
			allHealthy = false
		}
	}

	if hasFailure {
		return StateFailed
	}

	if allHealthy {
		return StateServing
	}

	// Still in progress
	return StateCreating
}

// SetupWithManager sets up the controller with the Manager
// Following production pattern: lower concurrency for stability
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.InferenceServer{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3, // Production uses lower concurrency
		}).
		Complete(r)
}
