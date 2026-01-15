// Package pipelinerun implements a Kubernetes controller for managing PipelineRun resources.
//
// The controller orchestrates the execution of machine learning pipelines by coordinating
// multiple stages through a condition-based engine:
//   - Source pipeline retrieval and validation
//   - Image building and management
//   - Workflow execution via Cadence/Temporal
//
// Each stage is implemented as a ConditionActor that checks prerequisites and executes
// actions. The controller manages state transitions and ensures consistent status updates
// for long-running pipeline executions.
package pipelinerun

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Reconciler implements the controller-runtime Reconciler interface for PipelineRun resources.
//
// It manages the execution lifecycle of pipeline runs through a condition-based engine,
// coordinating multiple actors (source pipeline, image build, workflow execution) to
// progress pipeline runs through their various states. The reconciler tracks execution
// status and updates the PipelineRun resource accordingly.
type Reconciler struct {
	api.Handler
	logger            *zap.Logger
	plugin            *plugin.Plugin
	engine            *defaultEngine.DefaultEngine[*v2pb.PipelineRun]
	apiHandlerFactory apiHandler.Factory
	notifier          *notification.PipelineRunNotifier
}

// NewReconciler creates a new PipelineRun controller reconciler.
//
// The reconciler is initialized with a condition-based engine that orchestrates
// pipeline execution through the provided plugin's actors. The logger is enhanced
// with component-specific fields for better observability.
//
// Parameters:
//   - plugin: Contains the ConditionActors for pipeline execution stages
//   - logger: Structured logger for the controller
//   - apiHandlerFactory: Factory for creating API handlers to interact with Kubernetes
//   - notifier: Handles pipeline run notifications for state changes
//
// Returns a configured Reconciler ready to be registered with a controller manager.
func NewReconciler(
	plugin *plugin.Plugin,
	logger *zap.Logger,
	apiHandlerFactory apiHandler.Factory,
	notifier *notification.PipelineRunNotifier,
) *Reconciler {
	logger = logger.With(zap.String("component", "pipelinerun"))
	return &Reconciler{
		plugin:            plugin,
		logger:            logger,
		engine:            defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		apiHandlerFactory: apiHandlerFactory,
		notifier:          notifier,
	}
}

// Reconcile is the main reconciliation loop entry point for PipelineRun resources.
//
// It processes reconciliation requests by running the pipeline through the condition
// engine, which executes registered actors in sequence. Based on the engine's results,
// it updates the PipelineRun state:
//   - RUNNING: Pipeline execution is in progress
//   - SUCCEEDED: All conditions satisfied successfully
//   - FAILED: One or more conditions failed
//   - KILLED: Pipeline was explicitly terminated
//
// The method ensures that status changes are persisted to Kubernetes and returns
// appropriate requeue results for ongoing executions.
//
// Returns a Result indicating requeue behavior and an error if reconciliation fails.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pipelineRun := &v2pb.PipelineRun{}
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline run starts")
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipelineRun); err != nil {
		return ctrl.Result{}, fmt.Errorf("get pipeline run %q: %w", req.NamespacedName, err)
	}
	originalPipelineRun := pipelineRun.DeepCopy()
	conditionResult, err := r.engine.Run(ctx, r.plugin, pipelineRun)
	result := conditionResult.Result
	var returnErr error
	if err != nil {
		logger.Error("Failed to run engine",
			zap.Error(err),
			zap.String("operation", "run_engine"),
			zap.String("namespace", req.Namespace),
			zap.String("name", req.Name))
		returnErr = fmt.Errorf("run engine for pipeline run %q: %w", req.NamespacedName, err)
	} else {
		if conditionResult.IsKilled {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
		} else if !conditionResult.IsTerminal {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_RUNNING
		} else if conditionResult.AreSatisfied {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_SUCCEEDED
		} else {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_FAILED
		}
	}

	// Send notifications for state changes (non-blocking)
	if notificationErr := r.notifier.NotifyOnStateChange(ctx, originalPipelineRun, pipelineRun); notificationErr != nil {
		logger.Warn("Failed to send notifications",
			zap.Error(notificationErr),
			zap.String("pipeline_run", req.NamespacedName.String()))
		// Don't fail reconciliation due to notification errors
	}

	if err = r.updatePipelineRunStatus(ctx, pipelineRun, originalPipelineRun); err != nil {
		if returnErr != nil {
			logger.Error("Failed to update pipeline run status", zap.Error(err))
			return result, fmt.Errorf("update pipeline run status for %q: %w (previous error: %w)", req.NamespacedName, err, returnErr)
		}
		logger.Error("Failed to update pipeline run status",
			zap.Error(err),
			zap.String("operation", "update_status"),
			zap.String("namespace", req.Namespace),
			zap.String("name", req.Name))
		returnErr = fmt.Errorf("update pipeline run status for %q: %w", req.NamespacedName, err)
	}
	return result, returnErr
}

// updatePipelineRunStatus persists PipelineRun status changes to Kubernetes.
//
// It performs a deep comparison between the original and updated status to avoid
// unnecessary API calls. Only when changes are detected is the status updated via
// the Kubernetes API.
//
// Returns an error if the status update fails.
func (r *Reconciler) updatePipelineRunStatus(ctx context.Context, pipelineRun *v2pb.PipelineRun, originalPipelineRun *v2pb.PipelineRun) error {
	if !reflect.DeepEqual(pipelineRun.Status, originalPipelineRun.Status) {
		if err := r.UpdateStatus(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update status for pipeline run %q: %w", pipelineRun.Name, err)
		}
	}
	return nil
}

// Register sets up the PipelineRun controller with the controller-runtime manager.
//
// It initializes the API handler from the factory and configures the controller
// to watch PipelineRun resources. The controller will reconcile all PipelineRun
// objects whenever they are created, updated, or when reconciliation is triggered.
//
// Returns an error if the API handler cannot be created or controller registration fails.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.PipelineRun{}).
		Complete(r)
}
