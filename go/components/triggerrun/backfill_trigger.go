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

type backfillTrigger struct {
	Log            logr.Logger
	WorkflowClient clientInterface.WorkflowClient
}

// NewBackfillTrigger returns a new backfillTrigger
func NewBackfillTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient) Runner {
	return &backfillTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
	}
}

// Run starts the backfill trigger
func (r *backfillTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateWorkflowID(triggerRun)
	opt := clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, parctically no timeout
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}
	domain := r.WorkflowClient.GetDomain()
	rid, err := getWorkflowOpenRunID(ctx, wid, r.WorkflowClient, domain)
	if err != nil {
		// Don't return error - continue to attempt StartWorkflow.
		// If workflow is already running, StartWorkflow will fail (handled below).
		// If workflow is not running, StartWorkflow will succeed.
		// The workflow ID prevents duplicate workflows from being created.
		log.Error(err, "failed to get open workflow execution",
			"operation", "get_workflow_runid",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
	}
	if rid != nil && *rid != "" {
		log.Info("backfill cadence workflow already running",
			"operation", "run_backfill_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid,
			"runId", *rid)
		return v2pb.TriggerRunStatus{
			State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
			ExecutionWorkflowId: *rid,
			LogUrl:              getWorkflowURL(wid, r.WorkflowClient.GetProvider()),
		}, nil
	}
	log.Info("starting backfill workflow",
		"operation", "start_workflow",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"workflowId", opt.ID,
		"taskList", opt.TaskList)
	exec, err := r.WorkflowClient.StartWorkflow(
		ctx, opt, "trigger.BackfillTrigger", CreateTriggerRequest{TriggerRun: triggerRun})
	if err != nil {
		log.Error(err, "failed to start backfill workflow",
			"operation", "start_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", opt.ID)
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("start workflow for backfill trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	r.Log.Info("backfill workflow enabled",
		"operation", "workflow_started",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"execution_id", exec.ID,
		"run_id", exec.RunID)
	return v2pb.TriggerRunStatus{
		State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
		ExecutionWorkflowId: exec.ID,
		LogUrl:              getWorkflowURL(wid, r.WorkflowClient.GetProvider()),
	}, nil
}

// Kill stops the backfill trigger
func (r *backfillTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	if triggerRun.Status.State != v2pb.TRIGGER_RUN_STATE_RUNNING {
		err := fmt.Errorf("cannot kill backfill trigger run in state: %s", &triggerRun.Status.State)
		log.Error(err, "kill backfill trigger run failed")
		return v2pb.TriggerRunStatus{
			State:        triggerRun.Status.State,
			ErrorMessage: err.Error(),
		}, err
	}
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// GetStatus gets the status of a running backfill trigger
func (r *backfillTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	return getAdhocRunWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}
