package triggerrun

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

type cronTrigger struct {
	Log            logr.Logger
	WorkflowClient clientInterface.WorkflowClient
}

// NewCronTrigger returns a new cronTrigger
func NewCronTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient) Runner {
	return &cronTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
	}
}

// Run starts the cron trigger
func (r *cronTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateWorkflowID(triggerRun)
	opt := clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, practically no timeout
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}
	domain := r.WorkflowClient.GetDomain()
	rid, err := getWorkflowOpenRunID(ctx, wid, r.WorkflowClient, domain)
	if err != nil {
		// log the error and continue
		log.Error(err, "failed to get open workflow execution")
	}
	if rid != nil && *rid != "" {
		log.Info("scheduled workflow already running",
			zap.String("workflowId", wid), zap.String("runId", *rid))
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	}
	log.Info("starting scheduled workflow", zap.Any("option", opt))
	exec, err := r.WorkflowClient.StartWorkflow(
		ctx, opt, "trigger.CronTrigger", CreateTriggerRequest{TriggerRun: triggerRun})
	if err != nil {
		return v2pb.TriggerRunStatus{
			ErrorMessage: err.Error(),
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
		}, err
	}
	r.Log.Info("scheduled workflow enabled",
		zap.Any("execution_id", exec.ID), zap.Any("run_id", exec.RunID))
	return v2pb.TriggerRunStatus{
		State:  v2pb.TRIGGER_RUN_STATE_RUNNING,
		LogUrl: getWorkflowURL(wid, r.WorkflowClient.GetProvider()),
	}, nil
}

// Kill kills (disables) the cron trigger
func (r *cronTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// GetStatus - TODO: implement GetStatus to get more information of a running triggerrun
func (r *cronTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	return getWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}
