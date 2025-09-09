package triggerrun

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

// CronTriggerContext is the struct to store cron trigger context while parsing from cadence cron trigger query result
type CronTriggerContext struct {
	DS            string                       `json:"DS,omitempty"`
	StartedAt     string                       `json:"StartedAt,omitempty"`
	TriggeredRuns map[string]map[string]string `json:"TriggeredRuns,omitempty"`
}

var (
	// ErrTriggerNotFound is returned when the trigger is not found in the original pipeline
	ErrTriggerNotFound = errors.New("trigger not found in original pipeline")

	// ErrNoCronSchedule is returned when no cron schedule is specified for trigger
	ErrNoCronSchedule = errors.New("no cron schedule specified for trigger")

	// ErrTriggerSpecNotFound is returned when the trigger spec is not found in the original pipeline
	ErrTriggerSpecNotFound = errors.New("trigger spec not found in original pipeline")

	// RetryLabel is used to store the number of retries for the trigger run status updates
	RetryLabel = "triggerrun/num-retry"
)

// util function to kill workflow execution for cron trigger
func killCadenceWorkflow(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient) (v2pb.TriggerRunStatus, error) {
	wid := generateCadenceWorkflowID(triggerRun)
	rid, err := getCadenceOpenRunID(ctx, wid, workflowClient)
	if err != nil {
		log.Error(err, "failed to get workflow execution info", zap.Any("triggerRun", triggerRun))
		return triggerRun.Status, err
	}
	if rid == "" {
		log.Info("no open execution, scheduled run already killed")
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
		return triggerRun.Status, nil
	}
	err = workflowClient.CancelWorkflow(ctx, wid, rid, "trigger killed")
	if err != nil {
		log.Error(err, "failed to cancel scheduled workflow",
			zap.String("workflowId", wid), zap.String("runId", rid))
		return triggerRun.Status, err
	}
	log.Info("scheduled workflow cancelled")
	triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
	return triggerRun.Status, nil
}

// util function to get workflow execution status for cron trigger
func getStatusCadenceWorkflow(ctx context.Context, triggerRun *v2pb.TriggerRun, log logr.Logger, workflowClient clientInterface.WorkflowClient) (v2pb.TriggerRunStatus, error) {
	wid := generateCadenceWorkflowID(triggerRun)
	rid, err := getCadenceOpenRunID(ctx, wid, workflowClient)
	if err != nil {
		log.Error(err, "failed to get workflow execution info", zap.Any("triggerRun", triggerRun))
		return v2pb.TriggerRunStatus{
			State:        triggerRun.Status.State,
			ErrorMessage: "failed to get workflow execution info: " + err.Error(),
		}, err
	}
	if rid == "" {
		log.Info("no open execution, scheduled run finished")
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED}, nil
	}

	execInfo, err := workflowClient.GetWorkflowExecutionInfo(ctx, wid, rid)
	if err != nil {
		log.Error(err, "failed to get workflow execution details", zap.String("workflowId", wid), zap.String("runId", rid))
		return v2pb.TriggerRunStatus{
			State:        triggerRun.Status.State,
			ErrorMessage: "failed to get workflow execution details: " + err.Error(),
		}, err
	}

	switch execInfo.Status {
	case clientInterface.WorkflowExecutionStatusRunning:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	case clientInterface.WorkflowExecutionStatusCompleted:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED}, nil
	case clientInterface.WorkflowExecutionStatusFailed:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED}, nil
	case clientInterface.WorkflowExecutionStatusCanceled, clientInterface.WorkflowExecutionStatusTerminated:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil
	default:
		return v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil
	}
}

// util function to get cadence workflow open execution runID
// Since OSS interface doesn't support listing workflows, we'll use a fixed runID approach
// In practice, this should be enhanced to properly track workflow executions
func getCadenceOpenRunID(ctx context.Context, wid string, workflowClient clientInterface.WorkflowClient) (string, error) {
	// For now, we'll try to get execution info with empty runID
	// This is a limitation of the current OSS interface
	// TODO: Implement proper workflow tracking or enhance the interface

	// Try to get workflow info - if it fails, assume no running workflow
	execInfo, err := workflowClient.GetWorkflowExecutionInfo(ctx, wid, "")
	if err != nil {
		// If we can't get info, assume no running workflow
		return "", nil
	}

	// If we found execution info and it's running, return the workflow ID as run ID
	if execInfo.Status == clientInterface.WorkflowExecutionStatusRunning {
		return wid, nil
	}

	return "", nil
}

// util function to generate cadence workflow ID
func generateCadenceWorkflowID(tr *v2pb.TriggerRun) string {
	return tr.Namespace + "." + tr.Name
}

// util function to get cadence workflow URL
func getCadenceWorkflowURL(wid string) string {
	// OSS Cadence Web UI configuration
	// Based on sandbox configuration: Cadence Web UI runs on port 8088
	domain := "default" // From OSS config: domain: default
	urlPath := fmt.Sprintf("/domains/%s/workflows/%s", domain, wid)

	// For local development (sandbox): localhost:8088
	// For K8s deployment: http://cadence-web:8088 or http://localhost:8088 (via port-forward)
	logURL := "http://localhost:8088"

	path, _ := url.PathUnescape(urlPath)
	return logURL + path
}

func isTerminateState(tr *v2pb.TriggerRun) bool {
	return tr.Status.State == v2pb.TRIGGER_RUN_STATE_FAILED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_KILLED || tr.Status.State == v2pb.TRIGGER_RUN_STATE_SUCCEEDED
}
