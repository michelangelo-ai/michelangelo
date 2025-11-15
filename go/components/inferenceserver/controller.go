package inferenceserver

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	logger   *zap.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Gateway  gateways.Gateway
	Plugins  plugins.PluginRegistry
}

// Reconcile handles InferenceServer CRD reconciliation
// Following production patterns: timeout management, structured logging, change detection
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Setup structured logging with trace context
	r.logger = r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	r.logger.Info("Reconciling inference server starts")

	// Set timeout for reconciliation - following production pattern
	reconcileCtx, cancel := context.WithTimeout(ctx, ReconcilerTimeout)
	defer cancel()

	// Use internal reconcile method with proper error handling
	result, err := r.reconcile(reconcileCtx, req.NamespacedName)
	// Production pattern: never return errors, use events instead
	if err != nil {
		r.logger.Error("Reconciliation failed", zap.Error(err))
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
func (r *Reconciler) reconcile(ctx context.Context, namespacedName client.ObjectKey) (ctrl.Result, error) {
	// Fetch the InferenceServer instance
	var inferenceServer v2pb.InferenceServer
	if err := r.Get(ctx, namespacedName, &inferenceServer); err != nil {
		if errors.IsNotFound(err) {
			// Clean up orphaned resources following Uber pattern
			r.logger.Info("InferenceServer resource not found, cleaning up any orphaned resources")
			orphanedServer := v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
				},
			}
			_, cleanupErr := r.handleDeletion(ctx, &orphanedServer)
			if cleanupErr != nil {
				r.logger.Error("Failed to clean up orphaned resources", zap.Error(cleanupErr))
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Deep copy for change detection (Uber production pattern)
	originalInferenceServer := inferenceServer.DeepCopy()

	// Update external details first (like Uber's plugin.UpdateDetails)
	err := r.updateExternalDetails(ctx, &inferenceServer)
	if err != nil {
		r.logger.Error("Failed to update external details, proceeding with reconciliation", zap.Error(err))
	}

	// Determine plugin based on deletion state
	var plugin plugins.Plugin
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		backendPlugin, pluginErr := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if pluginErr != nil {
			r.logger.Error("Failed to get backend plugin", zap.Error(pluginErr))
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
		plugin = backendPlugin.GetDeletionPlugin(&inferenceServer)
	} else {
		backendPlugin, pluginErr := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if pluginErr != nil {
			r.logger.Error("Failed to get backend plugin", zap.Error(pluginErr))
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
		plugin = backendPlugin.GetCreationPlugin()
	}

	// Run the plugin engine (like Uber's engine.Run)
	engine := plugins.NewEngine()
	conditionResult, err := engine.Run(ctx, r.logger, plugin, &inferenceServer)
	if err != nil {
		r.logger.Error("Plugin processing failed", zap.Error(err))
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil // Never return errors, use events instead (Uber pattern)
	}

	// Parse state from conditions (like Uber's plugin.ParseState)
	state := parseStateFromConditions(&inferenceServer)
	inferenceServer.Status.State = state

	// Only update if there are changes (Uber production pattern)
	if !reflect.DeepEqual(originalInferenceServer, &inferenceServer) {
		r.logger.Info("Updating inference server state",
			zap.String("oldState", originalInferenceServer.Status.State.String()),
			zap.String("newState", inferenceServer.Status.State.String()))

		// Update observed generation and timestamp
		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)

		if err := r.Status().Update(ctx, &inferenceServer); err != nil {
			r.logger.Error("Failed to update status", zap.Error(err))
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
func (r *Reconciler) updateExternalDetails(ctx context.Context, inferenceServer *v2pb.InferenceServer) error {
	// Skip if resource is being deleted
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Skip if we haven't attempted creation yet (still in initial CREATING state)
	if inferenceServer.Status.ObservedGeneration == 0 || inferenceServer.Status.State == StateCreating {
		return nil
	}

	// Get current status from gateway
	statusResp, err := r.Gateway.GetInfrastructureStatus(ctx, r.logger, gateways.GetInfrastructureStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
	if err != nil {
		// Don't fail reconciliation for status check errors
		r.logger.Info("Failed to get infrastructure status", zap.Error(err))
		return nil
	}

	// Update status based on external state
	if statusResp.Status.State != inferenceServer.Status.State {
		r.logger.Info("External state change detected",
			zap.String("currentState", inferenceServer.Status.State.String()),
			zap.String("externalState", statusResp.Status.State.String()))

		inferenceServer.Status.State = statusResp.Status.State
		inferenceServer.Status.ProviderMetadata = statusResp.Status.Message

		// Record state transition events
		switch statusResp.Status.State {
		case StateServing:
			r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, "CreationCompleted", "InferenceServer creation completed successfully")
		case StateFailed:
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, "CreationFailed", "InferenceServer creation failed")
		}
	}

	return nil
}

// handleCreation manages the creation and update lifecycle
func (r *Reconciler) handleCreation(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	logger.Info("Handling InferenceServer creation/update")

	// Add finalizer if not present (required for proper cleanup)
	if !controllerutil.ContainsFinalizer(inferenceServer, _inferenceServerCleanedUpFinalizer) {
		r.logger.Info("Adding finalizer for proper deletion handling")
		controllerutil.AddFinalizer(inferenceServer, _inferenceServerCleanedUpFinalizer)
		if err := r.Update(ctx, inferenceServer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		// Return early to let the finalizer update happen
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Initialize status if needed (first time setup)
	if inferenceServer.Status.ObservedGeneration == 0 {
		r.logger.Info("Initializing InferenceServer status")

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
		r.logger.Info("Spec generation changed, updating",
			zap.Int64("oldGeneration", inferenceServer.Status.ObservedGeneration),
			zap.Int64("newGeneration", inferenceServer.Generation))

		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		// Don't change state here, let the infrastructure update handle it
	}

	// Only create infrastructure if we're in creating state
	if inferenceServer.Status.State == StateCreating {
		r.logger.Info("Creating infrastructure using plugin system")

		// Get the appropriate plugin for this backend type
		plugin, err := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
		if err != nil {
			r.logger.Error("Failed to get plugin for backend type", zap.Error(err), zap.String("backendType", inferenceServer.Spec.BackendType.String()))
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
			r.logger.Error("Failed to execute creation actors", zap.Error(err))
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
			r.logger.Info("Configuring proxy for serving server")

			proxyErr := r.Gateway.ConfigureProxy(ctx, logger, gateways.ConfigureProxyRequest{
				InferenceServer: inferenceServer.Name,
				Namespace:       inferenceServer.Namespace,
				ModelName:       inferenceServer.Name, // Use server name as model name for now
				BackendType:     inferenceServer.Spec.BackendType,
			})
			if proxyErr != nil {
				r.logger.Error("Failed to configure proxy", zap.Error(proxyErr))
				// Don't fail the reconciliation for proxy errors
			} else {
				inferenceServer.Status.ProviderMetadata = "proxy-configured"
			}
		}
	}

	// Determine requeue strategy based on current state
	switch inferenceServer.Status.State {
	case StateCreating:
		r.logger.Info("InferenceServer still creating, requeuing")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	case StateServing:
		r.logger.Info("InferenceServer serving, steady state requeue")
		return ctrl.Result{RequeueAfter: SteadyStateRequeueAfter}, nil
	case StateFailed:
		r.logger.Info("InferenceServer failed, requeuing for retry")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	default:
		r.logger.Info("Unknown state, requeuing", zap.String("state", inferenceServer.Status.State.String()))
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}
}

// handleDeletion manages the deletion lifecycle
func (r *Reconciler) handleDeletion(ctx context.Context, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	r.logger.Info("Handling InferenceServer deletion")

	// Delete infrastructure using plugin system
	plugin, err := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
	if err != nil {
		r.logger.Error("Failed to get plugin for deletion", zap.Error(err), zap.String("backendType", inferenceServer.Spec.BackendType.String()))
		// If plugin not found, consider deletion complete
		r.logger.Info("Plugin not found, considering deletion complete")
		inferenceServer.Status.State = StateDeleted
		return ctrl.Result{}, nil
	}

	// Execute deletion actors
	deletionPlugin := plugin.GetDeletionPlugin(inferenceServer)
	engine := plugins.NewEngine()

	_, err = engine.Run(ctx, r.logger, deletionPlugin, inferenceServer)
	if err != nil {
		r.logger.Error("Failed to execute deletion actors", zap.Error(err))
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
