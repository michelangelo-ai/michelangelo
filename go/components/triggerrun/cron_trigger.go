package triggerrun

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	cadence2 "github.com/michelangelo-ai/michelangelo/go/components/triggerrun/cadence"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

var _cadenceWorkflowURLPath = "/domains/%s/prod17-phx/workflows?range=last-30-days&workflowId=%s"

// CronTriggerParams are params for instantiating the Cron Trigger
type CronTriggerParams struct {
	fx.In
	Log           logr.Logger
	CadenceClient clientInterface.WorkflowClient
}

// CronTriggerResult are the output of NewCronTrigger
type CronTriggerResult struct {
	fx.Out
	CronTrigger Runner `name:"cron-trigger"`
}

type cronTrigger struct {
	Log           logr.Logger
	CadenceClient clientInterface.WorkflowClient
}

// NewCronTrigger returns a new cronTrigger
func NewCronTrigger(params CronTriggerParams) CronTriggerResult {
	return CronTriggerResult{
		CronTrigger: &cronTrigger{
			Log:           params.Log,
			CadenceClient: params.CadenceClient,
		},
	}
}

// Run starts the cron trigger
func (r *cronTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateCadenceWorkflowID(triggerRun)
	opt := clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, practically no timeout
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}
	rid, err := getCadenceOpenRunID(ctx, wid, r.CadenceClient)
	if err != nil {
		// log the error and continue
		log.Error(err, "failed to get open workflow execution")
	}
	if rid != "" {
		log.Info("scheduled workflow already running",
			zap.String("workflowId", wid), zap.String("runId", rid))
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	}
	log.Info("starting scheduled workflow", zap.Any("option", opt))
	exec, err := r.CadenceClient.StartWorkflow(
		ctx, opt, "trigger.PipelineRunTrigger", cadence2.PipelineRunTriggerRequest{TriggerRun: triggerRun})
	if err != nil {
		return v2pb.TriggerRunStatus{
			ErrorMessage: err.Error(),
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
		}, err
	}
	r.Log.Info("scheduled cadence workflow enabled",
		zap.Any("execution_id", exec.ID), zap.Any("run_id", exec.RunID))
	return v2pb.TriggerRunStatus{
		State:  v2pb.TRIGGER_RUN_STATE_RUNNING,
		LogUrl: getCadenceWorkflowURL(wid),
	}, nil
}

// Kill kills (disables) the cron trigger
func (r *cronTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	return killCadenceWorkflow(ctx, triggerRun, log, r.CadenceClient)
}

// GetStatus - TODO: implement GetStatus to get more information of a running triggerrun
func (r *cronTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	return getStatusCadenceWorkflow(ctx, triggerRun, log, r.CadenceClient)
}
