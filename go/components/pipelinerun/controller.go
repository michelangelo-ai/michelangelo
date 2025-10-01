package pipelinerun

import (
	"context"
	"fmt"
	"reflect"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Reconciler struct {
	api.Handler
	logger            *zap.Logger
	plugin            *plugin.Plugin
	engine            *defaultEngine.DefaultEngine[*v2pb.PipelineRun]
	apiHandlerFactory apiHandler.Factory
}

func NewReconciler(plugin *plugin.Plugin, logger *zap.Logger, apiHandlerFactory apiHandler.Factory) *Reconciler {
	logger = logger.With(zap.String("component", "pipelinerun"))
	return &Reconciler{
		plugin:            plugin,
		logger:            logger,
		engine:            defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		apiHandlerFactory: apiHandlerFactory,
	}
}

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
	if err = r.updatePipelineRunStatus(ctx, pipelineRun, originalPipelineRun); err != nil {
		if returnErr != nil {
			logger.Error("Failed to update pipeline run status", zap.Error(err))
			return result, fmt.Errorf("update pipeline run status for %q: %w (previous error: %v)", req.NamespacedName, err, returnErr)
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

func (r *Reconciler) updatePipelineRunStatus(ctx context.Context, pipelineRun *v2pb.PipelineRun, originalPipelineRun *v2pb.PipelineRun) error {
	if !reflect.DeepEqual(pipelineRun.Status, originalPipelineRun.Status) {
		if err := r.UpdateStatus(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update status for pipeline run %q: %w", pipelineRun.Name, err)
		}
	}
	return nil
}

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
