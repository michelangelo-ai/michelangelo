package triggerrun

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TriggerType constants define the supported trigger types.
//
// These constants are used by GetTriggerType to determine which Runner implementation
// should handle a specific TriggerRun resource.
const (
	TriggerTypeCron       = "cron"        // Recurring workflows based on cron expressions
	TriggerTypeBackfill   = "backfill"    // One-time workflows for historical data processing
	TriggerTypeBatchRerun = "batch_rerun" // Bulk reprocessing of previously executed workflows
	TriggerTypeInterval   = "interval"    // Workflows triggered at fixed intervals
	TriggerTypeUnknown    = "unknown"     // Unknown or unsupported trigger type
)

// CreateTriggerRequest is a data transfer object for trigger workflow execution.
//
// This struct is passed as the workflow input when starting trigger workflows
// (both cron and backfill). It contains the complete TriggerRun specification
// needed for workflow execution.
type CreateTriggerRequest struct {
	TriggerRun *v2pb.TriggerRun // The TriggerRun resource containing execution parameters
}

// killWorkflow terminates a running workflow execution.
//
// This shared utility function is used by all Runner implementations to stop
// workflow execution. It performs the following operations:
//  1. Generate workflow ID from TriggerRun namespace and name
//  2. Query for open workflow execution using workflow client
//  3. Call TerminateWorkflow if execution is running
//  4. Return KILLED status regardless of whether workflow was running (idempotent)
//
// Returns State=KILLED on success. If no workflow is running, returns KILLED
// without error (idempotent behavior).
func killWorkflow(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	wid := generateWorkflowID(triggerRun)
	rid, err := getWorkflowOpenRunID(ctx, wid, workflowClient, domain)
	if err != nil {
		log.Error(err, "failed to get workflow execution info",
			"operation", "get_workflow_runid",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
		return triggerRun.Status, fmt.Errorf("get workflow execution info for trigger %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}
	if rid == nil || *rid == "" {
		log.Info("no open execution, scheduled run already killed")
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
		return triggerRun.Status, nil
	}
	err = workflowClient.TerminateWorkflow(ctx, wid, *rid, "trigger killed")
	if err != nil {
		log.Error(err, "failed to terminate scheduled workflow",
			"operation", "terminate_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid,
			"runId", *rid)
		return triggerRun.Status, fmt.Errorf("terminate workflow for trigger %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}
	log.Info("scheduled workflow terminated")
	triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
	return triggerRun.Status, nil
}

// getRecurringRunWorkflowStatus retrieves workflow status for recurring triggers (cron/interval).
//
// This function checks for open workflow executions and maps workflow states to
// TriggerRun states. For recurring triggers, workflows should remain running until
// explicitly terminated.
//
// State mapping:
//   - Open execution with valid timestamp → RUNNING
//   - Terminated/Canceled → KILLED (user-initiated termination)
//   - Failed/TimedOut → FAILED (execution failure)
//
// The function uses ListOpenWorkflow to find active executions, examining the
// execution time and status to determine the current state.
//
// Returns TriggerRunStatus with the current state and error message if applicable.
func getRecurringRunWorkflowStatus(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	wid := generateWorkflowID(triggerRun)
	execInfo, err := getWorkflowOpenExecution(ctx, wid, workflowClient, domain)
	if err != nil {
		log.Error(err, "failed to list open workflow for recurring run",
			"operation", "list_open_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
		return v2pb.TriggerRunStatus{
				State:        triggerRun.Status.State,
				ErrorMessage: "failed to list open workflow: " + err.Error(),
			}, fmt.Errorf("list open workflow for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	if execInfo != nil && !execInfo.ExecutionTime.IsZero() {
		execTs := execInfo.ExecutionTime
		log.Info("current recurring run execution time", "execution_ts", execTs)
		status := execInfo.Status
		// Terminated and Canceled are user-initiated actions, treat as KILLED
		if status == clientInterface.WorkflowExecutionStatusTerminated ||
			status == clientInterface.WorkflowExecutionStatusCanceled {
			log.Info("workflow was terminated or canceled",
				"operation", "get_workflow_status",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"workflowId", wid,
				"status", status)
			return v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_KILLED,
				ErrorMessage: fmt.Sprintf("workflow was terminated with state: %v", status),
			}, nil
		}
		// Failed and TimedOut are actual failures
		if status == clientInterface.WorkflowExecutionStatusFailed ||
			status == clientInterface.WorkflowExecutionStatusTimedOut {
			err := fmt.Errorf("workflow failed with state: %v", status)
			log.Error(err, "workflow failed",
				"operation", "get_workflow_status",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"workflowId", wid,
				"status", status)
			return v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
				ErrorMessage: err.Error(),
			}, err
		}
	}
	return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
}

// getAdhocRunWorkflowStatus retrieves workflow status for one-time triggers (backfill/batch rerun).
//
// This function queries the workflow engine for a specific workflow execution identified
// by ExecutionWorkflowId in the TriggerRun status. It maps workflow execution states to
// TriggerRun states.
//
// State mapping:
//   - Running → RUNNING
//   - Completed → SUCCEEDED
//   - Failed/TimedOut/Canceled/Terminated → FAILED
//
// The function uses GetWorkflowExecutionInfo to describe the workflow execution and
// determine its current state. Unlike recurring workflows, one-time workflows are
// expected to complete or fail rather than run indefinitely.
//
// Returns an error if ExecutionWorkflowId is empty or if the workflow cannot be described.
func getAdhocRunWorkflowStatus(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	var (
		execResponse *clientInterface.WorkflowExecutionInfo
		err          error
	)
	wid := triggerRun.Status.ExecutionWorkflowId
	if wid == "" {
		err = fmt.Errorf("execution workflow id is empty")
		log.Error(err, "failed to get workflow status",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name)
		return v2pb.TriggerRunStatus{
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			ErrorMessage: "failed to get workflow status: " + err.Error(),
		}, err
	}
	execResponse, err = workflowClient.GetWorkflowExecutionInfo(ctx, wid, "")
	if err != nil {
		log.Error(err, "failed to describe workflow execution",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
		return v2pb.TriggerRunStatus{
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			ErrorMessage: "failed to describe workflow execution: " + err.Error(),
		}, err
	}
	status := execResponse.Status
	switch status {
	case clientInterface.WorkflowExecutionStatusFailed,
		clientInterface.WorkflowExecutionStatusTimedOut,
		clientInterface.WorkflowExecutionStatusCanceled,
		clientInterface.WorkflowExecutionStatusTerminated:
		err := fmt.Errorf("workflow is terminated with state: %v", status)
		return v2pb.TriggerRunStatus{
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			ErrorMessage: err.Error(),
		}, err
	case clientInterface.WorkflowExecutionStatusCompleted:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED}, nil
	case clientInterface.WorkflowExecutionStatusRunning:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	default:
		err := fmt.Errorf("workflow is terminated with unknown state: %v", status)
		return v2pb.TriggerRunStatus{
			State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			ErrorMessage: err.Error(),
		}, err
	}
}

// getWorkflowOpenRunID retrieves the run ID of an open workflow execution.
//
// This function queries for open workflow executions matching the provided workflow ID
// and extracts the run ID if an execution is found. The run ID uniquely identifies a
// specific execution instance of a workflow.
//
// Returns:
//   - Non-nil string pointer: Run ID of the open execution
//   - nil: No open execution found (workflow not running or completed)
//   - error: Failed to query workflow engine
func getWorkflowOpenRunID(ctx context.Context, wid string, workflowClient clientInterface.WorkflowClient, domain string) (*string, error) {
	execution, err := getWorkflowOpenExecution(ctx, wid, workflowClient, domain)
	if err != nil {
		return nil, err
	}
	if execution == nil || execution.Execution == nil || execution.Execution.RunID == "" {
		return nil, nil
	}
	return &execution.Execution.RunID, nil
}

// getWorkflowOpenExecution retrieves open workflow execution information.
//
// This function queries the workflow engine for open (currently running) workflow
// executions matching the provided workflow ID. It uses ListOpenWorkflow with a time
// filter from epoch start to present to find matching executions.
//
// The function uses exponential backoff retry (max 3 attempts) to handle transient
// workflow engine errors.
//
// Returns:
//   - Non-nil WorkflowExecutionInfo: Information about the open execution
//   - nil: No open execution found (workflow not running or completed)
//   - error: Failed to query workflow engine after retries
func getWorkflowOpenExecution(ctx context.Context, wid string, workflowClient clientInterface.WorkflowClient, domain string) (*clientInterface.WorkflowExecutionInfo, error) {
	var (
		err      error
		response *clientInterface.ListOpenWorkflowExecutionsResponse
	)

	err = backoff.Retry(func() error {
		// earliest time: set to the start of the epoch (January 1, 1970)
		earliest := time.Unix(0, 0).UnixNano()
		current := time.Now().UnixNano()
		response, err = workflowClient.ListOpenWorkflow(ctx, clientInterface.ListOpenWorkflowExecutionsRequest{
			Domain: domain,
			ExecutionFilter: &clientInterface.ExecutionFilter{
				WorkflowID: wid,
			},
			StartTimeFilter: &clientInterface.StartTimeFilter{
				EarliestTime: &earliest,
				LatestTime:   &current,
			},
		})
		return err
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3))
	if err != nil {
		return nil, err
	}
	if len(response.Executions) == 0 {
		return nil, nil
	}
	return &response.Executions[0], nil
}

// generateWorkflowID creates a deterministic workflow ID from a TriggerRun resource.
//
// The workflow ID is constructed as "<namespace>.<name>" which ensures uniqueness
// within the workflow engine domain and allows idempotent workflow starts. Using
// the same workflow ID for repeated starts prevents duplicate workflow executions.
//
// Returns a string in the format "namespace.name".
func generateWorkflowID(tr *v2pb.TriggerRun) string {
	return tr.Namespace + "." + tr.Name
}

// getWorkflowURL constructs a web UI URL for workflow monitoring.
//
// This function generates URLs to access workflow execution details in either
// Cadence Web or Temporal Web UI. The URL format differs between providers:
//
// Temporal:
//   - Base URL: http://localhost:8080
//   - Path: /namespaces/{domain}/workflows/{workflowId}
//
// Cadence (default):
//   - Base URL: http://localhost:8088
//   - Path: /domains/{domain}/workflows/{workflowId}
//
// Note: These URLs are configured for local development. In production environments,
// the base URLs should be configured based on the actual Cadence/Temporal deployment.
//
// Returns a complete URL string for workflow monitoring.
func getWorkflowURL(wid string, provider string) string {
	domain := "default" // Default domain for both Cadence and Temporal
	var (
		logURL  string
		urlPath string
	)
	if provider == "temporal" {
		// Temporal Web UI configuration
		// For local development: localhost:8080
		logURL = "http://localhost:8080"
		urlPath = fmt.Sprintf("/namespaces/%s/workflows/%s", domain, wid)
	} else {
		// Cadence Web UI configuration (default)
		// For local development: localhost:8088
		logURL = "http://localhost:8088"
		urlPath = fmt.Sprintf("/domains/%s/workflows/%s", domain, wid)
	}
	path, _ := url.PathUnescape(urlPath)
	return logURL + path
}

// isTerminateState checks if a TriggerRun is in a terminal state.
//
// Terminal states are SUCCEEDED, FAILED, or KILLED. Once a TriggerRun reaches
// a terminal state, it should be marked immutable and no further reconciliation
// is required.
//
// Returns true if the TriggerRun is in a terminal state, false otherwise.
func isTerminateState(tr *v2pb.TriggerRun) bool {
	return tr.Status.State == v2pb.TRIGGER_RUN_STATE_FAILED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_KILLED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_SUCCEEDED
}

// GetTriggerType determines the trigger type from a TriggerRun specification.
//
// This function examines the TriggerRun spec to identify which trigger type should
// handle the resource. The determination is made by checking for specific fields
// in priority order:
//
//  1. BatchRerun: Spec.Trigger.BatchRerun is set
//  2. Backfill: Both Spec.StartTimestamp and Spec.EndTimestamp are set
//  3. Interval: Spec.Trigger.IntervalSchedule is set
//  4. Cron: Spec.Trigger.CronSchedule is set
//  5. Unknown: None of the above conditions match
//
// The returned type string is used by the Reconciler to select the appropriate
// Runner implementation.
//
// Returns one of the TriggerType constants (TriggerTypeCron, TriggerTypeBackfill,
// TriggerTypeInterval, TriggerTypeBatchRerun, or TriggerTypeUnknown).
func GetTriggerType(tr *v2pb.TriggerRun) string {
	if tr.Spec.Trigger.GetBatchRerun() != nil {
		return TriggerTypeBatchRerun
	}
	if tr.Spec.StartTimestamp != nil && tr.Spec.EndTimestamp != nil {
		return TriggerTypeBackfill
	}
	if tr.Spec.Trigger.GetIntervalSchedule() != nil {
		return TriggerTypeInterval
	}
	if tr.Spec.Trigger.GetCronSchedule() != nil {
		return TriggerTypeCron
	}
	return TriggerTypeUnknown
}

// pauseWorkflow pauses a recurring trigger workflow schedule.
//
// This function suspends workflow schedule execution for recurring triggers (cron/interval)
// to prevent new executions from being scheduled. The workflow schedule remains alive but inactive.
//
// Returns State=PAUSED on success. If the trigger is not recurring or schedule is not found,
// returns appropriate error state.
func pauseWorkflow(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	wid := generateWorkflowID(triggerRun)

	// Only cron and interval triggers can be paused (they use schedules)
	triggerType := GetTriggerType(triggerRun)
	if triggerType != TriggerTypeCron && triggerType != TriggerTypeInterval {
		log.Info("pause not supported for non-recurring trigger type", "triggerType", triggerType)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
		triggerRun.Status.ErrorMessage = fmt.Sprintf("pause operation not supported for trigger type: %s", triggerType)
		return triggerRun.Status, fmt.Errorf("pause not supported for trigger type %s", triggerType)
	}

	log.Info("pausing trigger", "workflowID", wid, "triggerType", triggerType)

	err := workflowClient.PauseTrigger(ctx, wid)
	if err != nil {
		log.Error(err, "failed to pause trigger",
			"operation", "pause_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowID", wid)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
		triggerRun.Status.ErrorMessage = err.Error()
		return triggerRun.Status, fmt.Errorf("pause trigger %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}

	log.Info("trigger paused successfully", "workflowID", wid)
	triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_PAUSED
	triggerRun.Status.ErrorMessage = ""
	return triggerRun.Status, nil
}

// resumeWorkflow resumes a paused recurring trigger workflow schedule.
//
// This function reactivates workflow schedule execution for previously paused recurring triggers,
// allowing new executions to be scheduled again according to the original schedule.
//
// Returns State=RUNNING on success. If the trigger is not recurring or schedule is not found,
// returns appropriate error state.
func resumeWorkflow(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	wid := generateWorkflowID(triggerRun)

	// Only cron and interval triggers can be resumed (they use schedules)
	triggerType := GetTriggerType(triggerRun)
	if triggerType != TriggerTypeCron && triggerType != TriggerTypeInterval {
		log.Info("resume not supported for non-recurring trigger type", "triggerType", triggerType)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
		triggerRun.Status.ErrorMessage = fmt.Sprintf("resume operation not supported for trigger type: %s", triggerType)
		return triggerRun.Status, fmt.Errorf("resume not supported for trigger type %s", triggerType)
	}

	log.Info("resuming trigger", "workflowID", wid, "triggerType", triggerType)

	err := workflowClient.UnpauseTrigger(ctx, wid)
	if err != nil {
		log.Error(err, "failed to resume trigger",
			"operation", "resume_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowID", wid)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
		triggerRun.Status.ErrorMessage = err.Error()
		return triggerRun.Status, fmt.Errorf("resume trigger %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}

	log.Info("trigger resumed successfully", "workflowID", wid)
	triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_RUNNING
	triggerRun.Status.ErrorMessage = ""
	return triggerRun.Status, nil
}
