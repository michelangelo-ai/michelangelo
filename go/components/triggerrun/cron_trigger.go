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
// to the workflow engine's CronSchedule option.
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
// This method performs the following operations:
//  1. Generate deterministic workflow ID from namespace and name
//  2. Check if workflow client supports native schedules
//  3. If schedules are supported (Temporal): Use StartScheduledWorkflow to create real schedule
//  4. If not supported (Cadence): Fall back to StartWorkflow with CronSchedule option
//  5. Return status with workflow URL for monitoring
//
// For Temporal with native schedules:
//   - Creates actual Temporal Schedule with cron expression
//   - Schedule automatically triggers workflows on schedule
//   - Better visibility and management in Temporal UI
//
// For Cadence or fallback mode:
//   - Uses traditional approach with long-running cron workflow
//   - Workflow manages cron scheduling internally
//
// Returns State=RUNNING if workflow/schedule starts successfully,
// State=FAILED if start fails.
func (r *cronTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateWorkflowID(triggerRun)

	// Check if workflow client supports native schedules (Temporal)
	if r.WorkflowClient.SupportsSchedules() {
		log.Info("using native schedule support",
			"operation", "start_schedule",
			"provider", r.WorkflowClient.GetProvider(),
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name)

		// Use new StartScheduledWorkflow method for native Temporal Schedules
		exec, err := r.WorkflowClient.StartScheduledWorkflow(ctx, clientInterface.ScheduledWorkflowOptions{
			TriggerRun:                      triggerRun,
			WorkflowType:                    "trigger.CronTrigger",
			TaskQueue:                       "trigger_run",
			Args:                            []interface{}{CreateTriggerRequest{TriggerRun: triggerRun}},
			ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, practically no timeout
			DecisionTaskStartToCloseTimeout: 30 * time.Second,
		})
		if err != nil {
			log.Error(err, "failed to start scheduled workflow",
				"operation", "start_schedule",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name)
			return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("start schedule for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
		}

		log.Info("scheduled workflow enabled via native schedule",
			"operation", "schedule_created",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"scheduleId", exec.ID)

		return v2pb.TriggerRunStatus{
			State:                v2pb.TRIGGER_RUN_STATE_RUNNING,
			ExecutionWorkflowId: exec.ID,
			LogUrl:              getWorkflowURL(exec.ID, r.WorkflowClient.GetProvider()),
		}, nil
	}

	// Fallback to traditional cron workflow for providers without native schedule support
	log.Info("using fallback cron workflow",
		"operation", "start_workflow_fallback",
		"provider", r.WorkflowClient.GetProvider(),
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name)

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

// Kill terminates a running cron-scheduled workflow or schedule.
//
// For providers with native schedule support (Temporal):
//   - Uses StopScheduledWorkflow to delete the actual schedule
//   - Prevents future scheduled executions from being triggered
//   - More efficient than terminating individual workflow executions
//
// For providers without native schedule support (Cadence):
//   - Falls back to traditional workflow termination
//   - Uses killWorkflow utility to terminate the cron workflow
//
// Returns State=KILLED on success. If no workflow/schedule is running, returns KILLED
// without error (idempotent termination).
func (r *cronTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})

	// Check if workflow client supports native schedules (Temporal)
	if r.WorkflowClient.SupportsSchedules() {
		log.Info("terminating via native schedule deletion",
			"operation", "stop_schedule",
			"provider", r.WorkflowClient.GetProvider(),
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name)

		scheduleID := fmt.Sprintf("%s-%s", triggerRun.Namespace, triggerRun.Name)
		err := r.WorkflowClient.StopScheduledWorkflow(ctx, scheduleID)
		if err != nil {
			log.Error(err, "failed to stop scheduled workflow",
				"operation", "stop_schedule",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"scheduleId", scheduleID)
			return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, err
		}

		log.Info("schedule deleted successfully",
			"operation", "schedule_deleted",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"scheduleId", scheduleID)

		return v2pb.TriggerRunStatus{
			State: v2pb.TRIGGER_RUN_STATE_KILLED,
		}, nil
	}

	// Fallback to traditional workflow termination
	log.Info("terminating via workflow termination",
		"operation", "kill_workflow_fallback",
		"provider", r.WorkflowClient.GetProvider(),
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name)

	domain := r.WorkflowClient.GetDomain()
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// GetStatus retrieves the execution status of a cron-scheduled workflow or schedule.
//
// For providers with native schedule support (Temporal):
//   - Uses GetScheduleStatus to check actual schedule state
//   - Maps schedule states to TriggerRun states:
//     - "RUNNING" → RUNNING
//     - "PAUSED" → RUNNING (still considered active)
//     - "FAILED" → FAILED
//     - Schedule not found → KILLED
//
// For providers without native schedule support (Cadence):
//   - Falls back to traditional workflow status checking
//   - Uses getRecurringRunWorkflowStatus for workflow state mapping
//
// Returns the current TriggerRunStatus with state and error information if applicable.
func (r *cronTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})

	// Check if workflow client supports native schedules (Temporal)
	if r.WorkflowClient.SupportsSchedules() {
		log.Info("checking status via native schedule",
			"operation", "get_schedule_status",
			"provider", r.WorkflowClient.GetProvider(),
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name)

		scheduleID := fmt.Sprintf("%s-%s", triggerRun.Namespace, triggerRun.Name)
		scheduleStatus, err := r.WorkflowClient.GetScheduleStatus(ctx, scheduleID)
		if err != nil {
			log.Error(err, "failed to get schedule status",
				"operation", "get_schedule_status",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"scheduleId", scheduleID)

			// If schedule not found, assume it was deleted/killed
			return v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_KILLED,
				ErrorMessage: err.Error(),
			}, nil
		}

		// Map schedule status to trigger run state
		var state v2pb.TriggerRunState
		switch scheduleStatus.State {
		case "RUNNING":
			state = v2pb.TRIGGER_RUN_STATE_RUNNING
		case "PAUSED":
			state = v2pb.TRIGGER_RUN_STATE_RUNNING // Still considered running, just paused
		case "FAILED":
			state = v2pb.TRIGGER_RUN_STATE_FAILED
		default:
			state = v2pb.TRIGGER_RUN_STATE_RUNNING // Default to running for unknown states
		}

		log.Info("schedule status retrieved",
			"operation", "get_schedule_status",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"scheduleState", scheduleStatus.State,
			"triggerState", state)

		return v2pb.TriggerRunStatus{
			State:        state,
			ErrorMessage: scheduleStatus.ErrorMessage,
		}, nil
	}

	// Fallback to traditional workflow status checking
	log.Info("checking status via workflow status",
		"operation", "get_workflow_status_fallback",
		"provider", r.WorkflowClient.GetProvider(),
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name)

	domain := r.WorkflowClient.GetDomain()
	return getRecurringRunWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}
