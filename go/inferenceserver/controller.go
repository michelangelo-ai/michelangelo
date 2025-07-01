package inferenceserver

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Reconciler reconciles InferenceServer objects
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Gateway  inferenceserver.Gateway
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

// reconcile is the internal reconciliation logic
func (r *Reconciler) reconcile(ctx context.Context, namespacedName client.ObjectKey, logger logr.Logger) (ctrl.Result, error) {
	// Fetch the InferenceServer instance
	var inferenceServer v2pb.InferenceServer
	if err := r.Get(ctx, namespacedName, &inferenceServer); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("InferenceServer resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get InferenceServer: %w", err)
	}

	// Production pattern: deep copy for change detection
	originalServer := inferenceServer.DeepCopy()
	
	// Enhanced structured logging
	logger = logger.WithValues(
		"name", inferenceServer.Name,
		"namespace", inferenceServer.Namespace,
		"generation", inferenceServer.Generation,
		"observedGeneration", inferenceServer.Status.ObservedGeneration,
		"state", inferenceServer.Status.State,
		"backendType", inferenceServer.Spec.BackendType,
	)

	logger.Info("Reconciling InferenceServer")

	// Handle deletion or creation based on timestamp
	var result ctrl.Result
	var err error
	
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		result, err = r.handleDeletion(ctx, logger, &inferenceServer)
	} else {
		result, err = r.handleCreation(ctx, logger, &inferenceServer)
	}
	
	if err != nil {
		return result, err
	}

	// Update external details via gateway AFTER creation attempts (similar to production plugin.UpdateDetails)
	if err := r.updateExternalDetails(ctx, logger, &inferenceServer); err != nil {
		logger.Error(err, "Failed to update external details (non-fatal)")
		// Don't fail reconciliation for status check errors
	}

	// Production pattern: optimistic updates - only update if changed
	if !reflect.DeepEqual(originalServer.Status, inferenceServer.Status) {
		logger.Info("Status changed, updating",
			"oldState", originalServer.Status.State,
			"newState", inferenceServer.Status.State)
		
		// Update timestamp
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)
		
		if err := r.Status().Update(ctx, &inferenceServer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
	} else {
		logger.V(1).Info("No status changes detected, skipping update")
	}

	return result, nil
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
	statusResp, err := r.Gateway.GetInfrastructureStatus(ctx, logger, inferenceserver.InfrastructureStatusRequest{
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
			r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonCreationCompleted, MessageCreationCompleted)
		case StateFailed:
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonCreationFailed, MessageCreationFailed)
		}
	}
	
	return nil
}

// handleCreation manages the creation and update lifecycle
func (r *Reconciler) handleCreation(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	logger.Info("Handling InferenceServer creation/update")
	
	// Initialize status if needed (first time setup)
	if inferenceServer.Status.ObservedGeneration == 0 {
		logger.Info("Initializing InferenceServer status")
		
		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.State = StateCreating
		inferenceServer.Status.Conditions = []*apipb.Condition{}
		inferenceServer.Status.CreateTime = time.Now().Format(time.RFC3339)
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)
		
		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonCreationStarted, MessageCreationStarted)
		
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
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonCreationFailed, fmt.Sprintf("No plugin available: %v", err))
			
			inferenceServer.Status.State = StateFailed
			inferenceServer.Status.ProviderMetadata = fmt.Sprintf("Plugin not found: %v", err)
			
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
		}
		
		// Execute creation actors
		actors := plugin.GetCreationActors()
		engine := plugins.NewActorEngine()
		
		err = engine.ExecuteActors(ctx, logger, inferenceServer, actors)
		if err != nil {
			logger.Error(err, "Failed to execute creation actors")
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonCreationFailed, fmt.Sprintf("%s: %v", MessageCreationFailed, err))
			
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
			
			proxyErr := r.Gateway.ConfigureProxy(ctx, logger, inferenceserver.ProxyConfigRequest{
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
	
	// Set state to deleting if not already
	if inferenceServer.Status.State != StateDeleting && inferenceServer.Status.State != StateDeleted {
		inferenceServer.Status.State = StateDeleting
		if err := r.Status().Update(ctx, inferenceServer); err != nil {
			logger.Error(err, "Failed to update status for deletion")
			return ctrl.Result{}, err
		}
		
		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonDeletionStarted, MessageDeletionStarted)
	}

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
	actors := plugin.GetDeletionActors()
	engine := plugins.NewActorEngine()
	
	err = engine.ExecuteActors(ctx, logger, inferenceServer, actors)
	if err != nil {
		logger.Error(err, "Failed to execute deletion actors")
		r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonDeletionFailed, fmt.Sprintf("%s: %v", MessageDeletionFailed, err))
		// Production pattern: don't return error, continue with requeue
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}

	// Check if deletion is complete by getting status
	statusResp, statusErr := r.Gateway.GetInfrastructureStatus(ctx, logger, inferenceserver.InfrastructureStatusRequest{
		InferenceServer: inferenceServer.Name,
		Namespace:       inferenceServer.Namespace,
		BackendType:     inferenceServer.Spec.BackendType,
	})
	
	// If infrastructure no longer exists, consider deletion complete
	if statusErr != nil || statusResp.State == StateDeleted {
		logger.Info("Deletion completed")
		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonDeletionCompleted, MessageDeletionCompleted)
		
		inferenceServer.Status.State = StateDeleted
		inferenceServer.Status.ProviderMetadata = "Infrastructure deletion completed"
		
		// Deletion complete, let Kubernetes clean up
		return ctrl.Result{}, nil
	}

	// Continue deletion process
	logger.V(1).Info("Deletion in progress, requeuing")
	return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
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