package triggerrun

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/cenkalti/backoff"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TriggerType constants for different trigger types
const (
	TriggerTypeCron       = "cron"
	TriggerTypeBackfill   = "backfill"
	TriggerTypeBatchRerun = "batch_rerun"
	TriggerTypeInterval   = "interval"
	TriggerTypeUnknown    = "unknown"
)

// CreateTriggerRequest DTO for the CreateTrigger workflow
type CreateTriggerRequest struct {
	TriggerRun *v2pb.TriggerRun
}

// util function to kill workflow execution for cron trigger
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

// util function to get workflow execution status for recurring run trigger
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

// util function to get workflow execution status for adhoc run trigger
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

// util function to get workflow open execution runID
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

// util function to get workflow open execution
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

// util function to generate workflow ID
func generateWorkflowID(tr *v2pb.TriggerRun) string {
	return tr.Namespace + "." + tr.Name
}

// util function to get workflow URL based on provider
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

// util function to check if the trigger run is in a terminate state
func isTerminateState(tr *v2pb.TriggerRun) bool {
	return tr.Status.State == v2pb.TRIGGER_RUN_STATE_FAILED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_KILLED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_SUCCEEDED
}

// GetTriggerType returns the trigger type for a given triggerRun
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
