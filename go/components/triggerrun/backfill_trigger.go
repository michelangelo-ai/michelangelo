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

// backfillTrigger implements the Runner interface for one-time backfill workflows.
//
// This implementation manages workflows that execute once to process historical data
// within a specified time range. Unlike cron triggers, backfill workflows complete
// after successful execution rather than running continuously.
//
// The time range for backfill is specified in TriggerRun.Spec.StartTimestamp and
// TriggerRun.Spec.EndTimestamp, which are passed to the workflow for processing.
type backfillTrigger struct {
	Log            logr.Logger                    // Structured logger for trigger operations
	WorkflowClient clientInterface.WorkflowClient // Workflow engine client (Cadence/Temporal)
}

// NewBackfillTrigger creates a new backfill trigger Runner.
//
// The returned Runner manages one-time workflows for historical data processing.
// It requires a logger for structured logging and a workflow client for interacting
// with the workflow engine.
func NewBackfillTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient) Runner {
	return &backfillTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
	}
}

// Run starts a one-time backfill workflow.
//
// This method performs the following operations:
//  1. Generate deterministic workflow ID from namespace and name
//  2. Check if workflow is already running (idempotent start)
//  3. Configure workflow options without cron schedule
//  4. Start workflow execution with "trigger.BackfillTrigger" workflow type
//  5. Return status with workflow ID and URL for monitoring
//
// The workflow uses:
//   - ID: <namespace>.<name> (deterministic for idempotency)
//   - TaskList: "trigger_run"
//   - ExecutionStartToCloseTimeout: 1 year (practically no timeout)
//   - DecisionTaskStartToCloseTimeout: 30 seconds
//   - No CronSchedule (one-time execution)
//
// Returns State=RUNNING if workflow starts successfully or is already running,
// State=FAILED if workflow start fails. The ExecutionWorkflowId field is populated
// with the workflow execution ID for status tracking.
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

// Kill terminates a running backfill workflow.
//
// This method stops the one-time workflow execution. It first validates that the
// workflow is in RUNNING state before attempting termination, returning an error
// if the workflow is not running.
//
// The workflow termination is handled by the shared killWorkflow utility function.
//
// Returns State=KILLED on success. Returns an error if the workflow is not in
// RUNNING state. If no workflow is running, returns KILLED without error
// (idempotent termination).
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

// GetStatus retrieves the execution status of a backfill workflow.
//
// This method queries the workflow engine for the specific workflow execution
// identified by ExecutionWorkflowId in the TriggerRun status. It maps workflow
// execution states to TriggerRun states.
//
// The status check is delegated to getAdhocRunWorkflowStatus which handles
// state mapping for one-time workflows:
//   - Running → RUNNING
//   - Completed → SUCCEEDED
//   - Failed/TimedOut/Canceled/Terminated → FAILED
//
// Returns an error if ExecutionWorkflowId is empty (workflow was never started)
// or if the workflow execution cannot be described.
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
