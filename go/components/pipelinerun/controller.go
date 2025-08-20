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
	"sigs.k8s.io/controller-runtime/pkg/controller"
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
		logger.Error("Failed to get pipeline run", zap.Error(err))
		return ctrl.Result{}, fmt.Errorf("failed to get pipeline run: %w", err)
	}
	originalPipelineRun := pipelineRun.DeepCopy()
	conditionResult, err := r.engine.Run(ctx, r.plugin, pipelineRun)
	result := conditionResult.Result
	var returnErr error
	if err != nil {
		logger.Error("Failed to run engine", zap.Error(err))
		returnErr = fmt.Errorf("Failed to run engine: %w", err)
	} else {
		if !conditionResult.IsTerminal {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_RUNNING
		} else if conditionResult.AreSatisfied {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_SUCCEEDED
		} else {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_FAILED
		}
	}
	if err = r.updatePipelineRunStatus(ctx, pipelineRun, originalPipelineRun); err != nil {
		logger.Error("Failed to update pipeline run status", zap.Error(err))
		returnErr = fmt.Errorf("Failed to update pipeline run status: %w %w", err, returnErr)
	}
	return result, returnErr
}

func (r *Reconciler) updatePipelineRunStatus(ctx context.Context, pipelineRun *v2pb.PipelineRun, originalPipelineRun *v2pb.PipelineRun) error {
	if !reflect.DeepEqual(pipelineRun.Status, originalPipelineRun.Status) {
		return r.UpdateStatus(ctx, pipelineRun, &metav1.UpdateOptions{})
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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 0,
			Reconciler:              nil,
			RateLimiter:             nil,
			LogConstructor:          nil,
			CacheSyncTimeout:        0,
			RecoverPanic:            false,
		}).
		For(&v2pb.PipelineRun{}).
		Complete(r)
}
