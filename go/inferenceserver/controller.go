package inferenceserver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Reconciler reconciles InferenceServer objects
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Provider Provider
}

// Reconcile handles InferenceServer CRD reconciliation
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("inferenceserver-controller")
	
	// Set timeout for reconciliation
	reconcileCtx, cancel := context.WithTimeout(ctx, ReconcilerTimeout)
	defer cancel()
	
	// Fetch the InferenceServer instance
	var inferenceServer v2pb.InferenceServer
	if err := r.Get(reconcileCtx, req.NamespacedName, &inferenceServer); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("InferenceServer resource not found, ignoring deletion")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get InferenceServer")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling InferenceServer", 
		"name", inferenceServer.Name,
		"namespace", inferenceServer.Namespace,
		"generation", inferenceServer.Generation,
		"observedGeneration", inferenceServer.Status.ObservedGeneration)

	// Handle deletion
	if !inferenceServer.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(reconcileCtx, logger, &inferenceServer)
	}

	// Handle creation/update
	return r.handleCreation(reconcileCtx, logger, &inferenceServer)
}

// handleCreation manages the creation and update lifecycle
func (r *Reconciler) handleCreation(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	logger.Info("Handling InferenceServer creation/update")
	
	// Initialize status if needed
	if inferenceServer.Status.ObservedGeneration == 0 {
		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.State = StateCreating
		inferenceServer.Status.Conditions = []*apipb.Condition{}
		
		if err := r.Status().Update(ctx, inferenceServer); err != nil {
			logger.Error(err, "Failed to initialize status")
			return ctrl.Result{}, err
		}
		
		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonCreationStarted, MessageCreationStarted)
	}

	// Create infrastructure via provider
	_, err := r.Provider.Create(ctx, &CreateRequest{
		InferenceServer: inferenceServer,
		Logger:          logger,
	})
	if err != nil {
		logger.Error(err, "Failed to create infrastructure")
		r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonCreationFailed, fmt.Sprintf("%s: %v", MessageCreationFailed, err))
		
		// Update status to failed
		inferenceServer.Status.State = StateFailed
		inferenceServer.Status.ProviderMetadata = fmt.Sprintf("Creation failed: %v", err)
		r.Status().Update(ctx, inferenceServer)
		
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
	}

	// Get current status from provider
	getResp, err := r.Provider.Get(ctx, &GetRequest{
		InferenceServer: inferenceServer,
		Logger:          logger,
	})
	if err != nil {
		logger.Error(err, "Failed to get status from provider")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
	}

	// Update status based on provider response
	currentState := inferenceServer.Status.State
	newState := getResp.State
	
	if currentState != newState {
		logger.Info("State transition", "from", currentState, "to", newState)
		inferenceServer.Status.State = newState
		inferenceServer.Status.ProviderMetadata = getResp.Message
		
		// Record appropriate event
		switch newState {
		case StateServing:
			r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonCreationCompleted, MessageCreationCompleted)
		case StateFailed:
			r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonCreationFailed, MessageCreationFailed)
		}
		
		// Update status
		if err := r.Status().Update(ctx, inferenceServer); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
	}

	// Determine requeue strategy
	switch inferenceServer.Status.State {
	case StateCreating:
		logger.Info("InferenceServer still creating, requeuing")
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	case StateServing:
		logger.Info("InferenceServer serving, steady state requeue")
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

	// Delete infrastructure via provider
	deleteResp, err := r.Provider.Delete(ctx, &DeleteRequest{
		InferenceServer: inferenceServer,
		Logger:          logger,
	})
	if err != nil {
		logger.Error(err, "Failed to delete infrastructure")
		r.Recorder.Event(inferenceServer, corev1.EventTypeWarning, EventReasonDeletionFailed, fmt.Sprintf("%s: %v", MessageDeletionFailed, err))
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
	}

	// Update status based on deletion response
	if deleteResp.State == StateDeleted {
		logger.Info("Deletion completed")
		r.Recorder.Event(inferenceServer, corev1.EventTypeNormal, EventReasonDeletionCompleted, MessageDeletionCompleted)
		
		inferenceServer.Status.State = StateDeleted
		inferenceServer.Status.ProviderMetadata = deleteResp.Message
		
		if err := r.Status().Update(ctx, inferenceServer); err != nil {
			logger.Error(err, "Failed to update deletion status")
			return ctrl.Result{}, err
		}
		
		// Deletion complete, let Kubernetes clean up
		return ctrl.Result{}, nil
	}

	// Continue deletion process
	logger.Info("Deletion in progress, requeuing")
	return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.InferenceServer{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}