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

// util function to get workflow execution status for cron trigger
func getWorkflowStatus(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient, domain string) (v2pb.TriggerRunStatus, error) {
	wid := generateWorkflowID(triggerRun)
	execInfo, err := getWorkflowOpenExecution(ctx, wid, workflowClient, domain)
	if err != nil {
		log.Error(err, "failed to list open workflow for scheduled run",
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
		log.Info("current scheduled execution time", "execution_ts", execTs)
	}
	return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
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
	urlPath := ""
	switch provider {
	case "temporal":
		// Temporal Web UI configuration
		// For local development: localhost:8080
		urlPath = fmt.Sprintf("/namespaces/%s/workflows/%s", domain, wid)
	case "cadence":
		// Cadence Web UI configuration
		// For local development: localhost:8088
		urlPath = fmt.Sprintf("/domains/%s/workflows/%s", domain, wid)
	default:
		// Default to Cadence format
		urlPath = fmt.Sprintf("/domains/%s/workflows/%s", domain, wid)
	}
	logURL := "http://localhost:8088"
	path, _ := url.PathUnescape(urlPath)
	return logURL + path
}

// util function to check if the trigger run is in a terminate state
func isTerminateState(tr *v2pb.TriggerRun) bool {
	return tr.Status.State == v2pb.TRIGGER_RUN_STATE_FAILED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_KILLED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_SUCCEEDED
}

// GetTriggerType returns the trigger type for a given triggerRun
func GetTriggerType(tr *v2pb.TriggerRun) string {
	if tr == nil || tr.Spec.Trigger == nil {
		return TriggerTypeUnknown
	}

	trigger := tr.Spec.Trigger
	if trigger.GetCronSchedule() != nil {
		return TriggerTypeCron
	}
	if trigger.GetIntervalSchedule() != nil {
		return TriggerTypeInterval
	}
	if trigger.GetBatchRerun() != nil {
		return TriggerTypeBatchRerun
	}
	if tr.Spec.StartTimestamp != nil && tr.Spec.EndTimestamp != nil {
		return TriggerTypeBackfill
	}

	return TriggerTypeUnknown
}
