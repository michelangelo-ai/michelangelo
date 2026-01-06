// Package inferenceserver provides Kubernetes controller logic for managing ML inference servers.
//
// The inferenceserver controller manages the complete lifecycle of inference serving infrastructure,
// including provisioning, model management, network routing, and cleanup. It provides a unified
// interface for multiple inference backends through a plugin architecture.
//
//   - NVIDIA Triton Inference Server
//
// Infrastructure Management:
//   - Creates Kubernetes Deployments, Services, and ConfigMaps
//   - Manages resource allocation (CPU, GPU, memory)
//   - Handles scaling and replica management
//
// Network Routing:
//   - Sets up URL rewriting rules
//   - Manages traffic routing for deployments
//
// Health & Status:
//   - Performs health checks on inference servers
//   - Reports infrastructure and model status
//   - Updates CRD status with current state
//
// The controller follows a condition-based lifecycle model:
//
//	Creation:  Validation → Resource Creation → Health Check
//	Deletion:  Cleanup → Resource Removal
package inferenceserver

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Reconciler reconciles InferenceServer custom resources and manages their lifecycle.
//
// The Reconciler handles:
//   - Infrastructure provisioning (Kubernetes deployments, services, configmaps)
//   - Health checking and status reporting
//   - Cleanup and finalization on deletion
type Reconciler struct {
	api.Handler

	logger            *zap.Logger
	Recorder          record.EventRecorder
	engine            conditionInterfaces.Engine[*v2pb.InferenceServer]
	Plugins           plugins.PluginRegistry
	apiHandlerFactory apiHandler.Factory
}

// NewReconciler creates a new InferenceServer reconciler with the required dependencies.
//
// The reconciler manages the complete lifecycle of inference servers including infrastructure
// creation, model management, and network routing configuration.
//
// Parameters:
//   - mgr: The controller-runtime manager providing Kubernetes client access
//   - scheme: Kubernetes runtime scheme with registered CRD types
//   - pluginRegistry: Registry of backend-specific plugins
//   - apiHandlerFactory: Factory for creating API handlers
//   - logger: Structured logger for observability
//
// Returns:
//   - *Reconciler: Configured reconciler ready to be registered with the manager
func NewReconciler(mgr ctrl.Manager, scheme *runtime.Scheme, pluginRegistry plugins.PluginRegistry, apiHandlerFactory apiHandler.Factory, logger *zap.Logger) *Reconciler {
	logger = logger.With(zap.String("component", "inferenceserver"))
	return &Reconciler{
		engine:            defaultengine.NewDefaultEngine[*v2pb.InferenceServer](logger),
		Recorder:          mgr.GetEventRecorderFor(ControllerName),
		Plugins:           pluginRegistry,
		apiHandlerFactory: apiHandlerFactory,
		logger:            logger,
	}
}

// SetupWithManager registers the reconciler with the controller manager.
//
// This method:
//   - Initializes the API handler for Kubernetes operations
//   - Configures the controller to watch InferenceServer resources
//   - Sets concurrency limits (3 concurrent reconciles for stability)
//   - Registers event handlers and predicates
//
// Parameters:
//   - mgr: The controller-runtime manager to register with
//
// Returns:
//   - error: nil on success, error if registration fails
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the Handler using the apiHandlerFactory
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return fmt.Errorf("failed to create API handler: %w", err)
	}
	r.Handler = handler

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.InferenceServer{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3, // Production uses lower concurrency
		}).
		Complete(r)
}

// Reconcile implements the main reconciliation loop for InferenceServer resources.
//
// The reconciliation process handles:
//  1. Resource retrieval and validation
//  2. Plugin selection based on backend type
//  3. Deletion handling (cleanup phase) OR Creation/Update handling (provision phase)
//  4. Condition engine execution for lifecycle management
//  5. Status updates and condition synchronization
//
// Parameters:
//   - ctx: Context for cancellation and deadlines
//   - req: Reconciliation request with namespaced name
//
// Returns:
//   - ctrl.Result: Requeue configuration (duration, immediate, etc.)
//   - error: nil on success, error if reconciliation fails
//
// Requeue behavior:
//   - Active operations: Requeue after 1 minute
//   - Steady state: Requeue after 10 minutes
//   - Errors: Exponential backoff via controller-runtime
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Setup structured logging with trace context
	r.logger = r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	r.logger.Info("Reconciling inference server starts")

	// Set timeout for reconciliation, following production pattern
	reconcileCtx, cancel := context.WithTimeout(ctx, ReconcilerTimeout)
	defer cancel()

	// Fetch the InferenceServer instance
	var inferenceServer v2pb.InferenceServer
	if err := r.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name, &metav1.GetOptions{}, &inferenceServer); err != nil {
		if utils.IsNotFoundError(err) {
			r.logger.Error("request made for inference server that is not found. Ignoring this request",
				zap.Error(err),
				zap.String("operation", "get_inference_server"),
				zap.String("namespace", req.NamespacedName.Namespace),
				zap.String("name", req.NamespacedName.Name))
			return ctrl.Result{}, nil
		}

		r.logger.Error("failed to retrieve inference server object",
			zap.Error(err),
			zap.String("operation", "get_inference_server"),
			zap.String("namespace", req.NamespacedName.Namespace),
			zap.String("name", req.NamespacedName.Name))
		return ctrl.Result{}, err
	}

	// Use internal reconcile method, with proper error handling
	result, err := r.reconcile(reconcileCtx, &inferenceServer)
	// Production pattern: never return errors, use events instead
	if err != nil {
		// Record event for user visibility
		r.Recorder.Event(&inferenceServer, corev1.EventTypeWarning, "ReconciliationError", err.Error())
		// Return success to avoid exponential backoff
		return result, nil
	}

	return result, nil
}

// reconcile does the work of deciding if we wish to perform an action upon the
// inference server to match our desired state.
func (r *Reconciler) reconcile(ctx context.Context, inferenceServer *v2pb.InferenceServer) (ctrl.Result, error) {
	// Deep copy for change detection (Uber production pattern)
	originalInferenceServer := inferenceServer.DeepCopy()

	// Determine plugin based on backend type
	backendPlugin, err := r.Plugins.GetPlugin(inferenceServer.Spec.BackendType)
	if err != nil {
		r.logger.Error("Failed to get backend plugin",
			zap.Error(err),
			zap.String("operation", "get_backend_plugin"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("name", inferenceServer.Name),
			zap.String("backendType", inferenceServer.Spec.BackendType.String()))
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
	}

	err = backendPlugin.UpdateDetails(ctx, inferenceServer)
	if err != nil {
		r.logger.Error("Failed to update external details, proceeding with reconciliation",
			zap.Error(err),
			zap.String("operation", "update_external_details"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("name", inferenceServer.Name))
	}

	var conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]
	if !inferenceServer.GetDeletionTimestamp().IsZero() || isDecommissioned(inferenceServer) {
		conditionPlugin = backendPlugin.GetDeletionPlugin(inferenceServer)
	} else {
		conditionPlugin = backendPlugin.GetCreationPlugin()
	}

	// update inferenceServer.status.conditions with the conditions specific to the current conditionPlugin
	backendPlugin.UpdateConditions(inferenceServer, conditionPlugin)

	conditionResult, err := r.engine.Run(ctx, conditionPlugin, inferenceServer)
	if err != nil {
		r.logger.Error("Plugin processing failed",
			zap.Error(err),
			zap.String("operation", "run_condition_engine"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("name", inferenceServer.Name))
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
	}

	state := backendPlugin.ParseState(inferenceServer)
	inferenceServer.Status.State = state

	// Only update if there are changes (Uber production pattern)
	if !reflect.DeepEqual(originalInferenceServer, inferenceServer) {
		r.logger.Info("Updating inference server state",
			zap.String("oldState", originalInferenceServer.Status.State.String()),
			zap.String("newState", inferenceServer.Status.State.String()))

		// We copy the inference server at this point because the r.Client.Update call below will set the Status object
		// to an empty struct.
		inferenceServerCopy := inferenceServer.DeepCopy()
		err = r.Update(ctx, inferenceServer, &metav1.UpdateOptions{})
		if err != nil {
			r.logger.Error("Updating Inference Server metadata during reconcile failed",
				zap.Error(err),
				zap.String("operation", "update_metadata"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("name", inferenceServer.Name))
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
		}

		// persist the new status.
		inferenceServer.Status = inferenceServerCopy.Status
		inferenceServer.Status.ObservedGeneration = inferenceServer.Generation
		inferenceServer.Status.UpdateTime = time.Now().Format(time.RFC3339)

		err = r.UpdateStatus(ctx, inferenceServer, &metav1.UpdateOptions{})
		if err != nil {
			r.logger.Error("Updating Inference Server status during reconcile failed",
				zap.Error(err),
				zap.String("operation", "update_status"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("name", inferenceServer.Name))
			return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, err
		}
		r.logger.Info("Reconcile successfully updates Inference Server state")
	}

	// Convert condition result to appropriate requeue strategy
	if !conditionResult.AreSatisfied {
		// Continue active monitoring if there are failing conditions
		return ctrl.Result{RequeueAfter: ActiveRequeueAfter}, nil
	}

	// Use steady state requeue if everything is healthy
	return ctrl.Result{RequeueAfter: SteadyStateRequeueAfter}, nil
}

func isDecommissioned(inferenceServer *v2pb.InferenceServer) bool {
	return inferenceServer.Spec.DecomSpec != nil && inferenceServer.Spec.DecomSpec.Decommission
}
