package triggerrun

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// cronTrigger implements the Runner interface for cron-scheduled recurring workflows.
//
// This implementation manages workflows that execute on a recurring schedule defined by
// cron expressions. The workflow continues running until explicitly killed, spawning
// child workflow executions at each scheduled interval.
//
// The cron schedule is read from TriggerRun.Spec.Trigger.CronSchedule.Cron and passed
// to the workflow engine's StartWorkflow method with CronSchedule option.
type cronTrigger struct {
	Log            logr.Logger                    // Structured logger for trigger operations
	WorkflowClient clientInterface.WorkflowClient // Workflow engine client (Cadence/Temporal)
}

// NewCronTrigger creates a new cron trigger Runner.
//
// The returned Runner manages recurring scheduled workflows using cron expressions.
// It requires a logger for structured logging and a workflow client for interacting
// with the workflow engine.
func NewCronTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient) Runner {
	return &cronTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
	}
}

// Run starts a recurring cron-scheduled workflow.
//
// This method uses the WorkflowClient.StartWorkflow method with CronSchedule option.
// The client implementation will automatically decide whether to use native schedules
// (Temporal) or traditional cron workflows (Cadence) based on provider capabilities.
//
// Returns State=RUNNING if workflow/schedule starts successfully,
// State=FAILED if start fails.
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
		CronSchedule:                    triggerRun.Spec.Trigger.GetCronSchedule().GetCron(),
	}
	domain := r.WorkflowClient.GetDomain()
	rid, err := getWorkflowOpenRunID(ctx, wid, r.WorkflowClient, domain)
	if err != nil {
		// log the error and continue
		log.Error(err, "failed to get open workflow execution",
			"operation", "get_workflow_runid",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
	}
	if rid != nil && *rid != "" {
		log.Info("scheduled workflow already running",
			"operation", "run_cron_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid,
			"runId", *rid)
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	}
	log.Info("starting scheduled workflow",
		"operation", "start_workflow",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"workflowId", opt.ID,
		"taskList", opt.TaskList)
	exec, err := r.WorkflowClient.StartWorkflow(
		ctx, opt, "trigger.CronTrigger", CreateTriggerRequest{TriggerRun: triggerRun})
	if err != nil {
		log.Error(err, "failed to start scheduled workflow",
			"operation", "start_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", opt.ID)
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("start workflow for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	r.Log.Info("scheduled workflow enabled",
		"operation", "workflow_started",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"execution_id", exec.ID,
		"run_id", exec.RunID)
	return v2pb.TriggerRunStatus{
		State:  v2pb.TRIGGER_RUN_STATE_RUNNING,
		LogUrl: getWorkflowURL(wid, r.WorkflowClient.GetProvider()),
	}, nil
}

// Kill terminates a running cron-scheduled workflow.
//
// Uses the WorkflowClient to terminate workflows. The client implementation
// will automatically handle whether to delete schedules (Temporal) or
// terminate traditional workflows (Cadence).
//
// Returns State=KILLED on success. If no workflow is running, returns KILLED
// without error (idempotent termination).
func (r *cronTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})

	domain := r.WorkflowClient.GetDomain()
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// GetStatus retrieves the execution status of a cron-scheduled workflow.
//
// Uses the WorkflowClient to get workflow status. The client implementation
// will automatically handle whether to check schedule status (Temporal) or
// traditional workflow status (Cadence).
//
// Returns the current TriggerRunStatus with state and error information if applicable.
func (r *cronTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})

	domain := r.WorkflowClient.GetDomain()
	return getRecurringRunWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}
