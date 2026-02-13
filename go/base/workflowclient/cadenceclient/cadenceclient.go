package cadenceclient

import (
	"context"
	"fmt"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"go.uber.org/cadence/.gen/go/shared"
	cadenceClient "go.uber.org/cadence/client"
)

// mapCadenceStatusToInterface maps Cadence workflow status to our interface status
func mapCadenceStatusToInterface(closeStatus *shared.WorkflowExecutionCloseStatus) clientInterface.WorkflowExecutionStatus {
	if closeStatus == nil {
		return clientInterface.WorkflowExecutionStatusRunning
	}

	switch *closeStatus {
	case shared.WorkflowExecutionCloseStatusCompleted:
		return clientInterface.WorkflowExecutionStatusCompleted
	case shared.WorkflowExecutionCloseStatusFailed:
		return clientInterface.WorkflowExecutionStatusFailed
	case shared.WorkflowExecutionCloseStatusCanceled:
		return clientInterface.WorkflowExecutionStatusCanceled
	case shared.WorkflowExecutionCloseStatusTerminated:
		return clientInterface.WorkflowExecutionStatusTerminated
	case shared.WorkflowExecutionCloseStatusContinuedAsNew:
		return clientInterface.WorkflowExecutionStatusContinuedAsNew
	case shared.WorkflowExecutionCloseStatusTimedOut:
		return clientInterface.WorkflowExecutionStatusTimedOut
	default:
		return clientInterface.WorkflowExecutionStatusUnSpecified
	}
}

type CadenceClient struct {
	Client   cadenceClient.Client
	Provider string
	Domain   string
}

// ensure CadenceClient implements clientInterface.WorkflowClient
var _ clientInterface.WorkflowClient = &CadenceClient{}

func (c *CadenceClient) StartWorkflow(ctx context.Context, options clientInterface.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterface.WorkflowExecution, error) {

	cadenceOptions := cadenceClient.StartWorkflowOptions{
		ID:                              options.ID,
		TaskList:                        options.TaskList,
		ExecutionStartToCloseTimeout:    options.ExecutionStartToCloseTimeout,
		DecisionTaskStartToCloseTimeout: options.DecisionTaskStartToCloseTimeout,
		WorkflowIDReusePolicy:           cadenceClient.WorkflowIDReusePolicyAllowDuplicate,
		CronSchedule:                    options.CronSchedule,
	}
	cadenceWorkflowExecution, err := c.Client.StartWorkflow(ctx, cadenceOptions, workflowName, args...)
	if err != nil {
		return nil, err
	}
	workflowExecution := clientInterface.WorkflowExecution{
		ID:    cadenceWorkflowExecution.ID,
		RunID: cadenceWorkflowExecution.RunID,
	}
	return &workflowExecution, nil
}

func (c *CadenceClient) GetWorkflowExecutionInfo(ctx context.Context, workflowID string, runID string) (*clientInterface.WorkflowExecutionInfo, error) {
	describeWorkflowExecutionResponse, err := c.Client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	cadenceWorkflowExecutionInfo := describeWorkflowExecutionResponse.WorkflowExecutionInfo
	if cadenceWorkflowExecutionInfo == nil {
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusUnSpecified,
		}, nil
	}

	var closeStatus *shared.WorkflowExecutionCloseStatus
	if cadenceWorkflowExecutionInfo.IsSetCloseStatus() {
		status := cadenceWorkflowExecutionInfo.GetCloseStatus()
		closeStatus = &status
	}

	return &clientInterface.WorkflowExecutionInfo{
		Status: mapCadenceStatusToInterface(closeStatus),
	}, nil
}

func (c *CadenceClient) CancelWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	cancelWorkflowOptions := cadenceClient.WithCancelReason(reason)
	return c.Client.CancelWorkflow(ctx, workflowID, runID, cancelWorkflowOptions)
}

func (c *CadenceClient) QueryWorkflow(ctx context.Context, workflowID string, runID string, queryHandlerKey string, queryResult any) error {
	queryWorkflowWithOptionRequest := cadenceClient.QueryWorkflowWithOptionsRequest{
		WorkflowID:            workflowID,
		RunID:                 runID,
		QueryType:             queryHandlerKey,
		QueryConsistencyLevel: shared.QueryConsistencyLevelStrong.Ptr(),
	}
	queryWorkflowWithOptionResponse, err := c.Client.QueryWorkflowWithOptions(ctx, &queryWorkflowWithOptionRequest)
	if err != nil || queryWorkflowWithOptionResponse == nil {
		return fmt.Errorf("error querying workflow workflowID %s, runID %s for queryHandlerKey %s with Error: %w", workflowID, runID, queryHandlerKey, err)
	}
	if queryWorkflowWithOptionResponse.QueryResult.HasValue() {
		if err := queryWorkflowWithOptionResponse.QueryResult.Get(&queryResult); err != nil {
			return fmt.Errorf("error getting query result: %w", err)
		}
	}
	return nil
}

func (c *CadenceClient) GetProvider() string {
	return c.Provider
}

func (c *CadenceClient) GetDomain() string {
	return c.Domain
}

func (c *CadenceClient) ListOpenWorkflow(ctx context.Context, request clientInterface.ListOpenWorkflowExecutionsRequest) (*clientInterface.ListOpenWorkflowExecutionsResponse, error) {
	cadenceRequest := &shared.ListOpenWorkflowExecutionsRequest{
		Domain:          &request.Domain,
		MaximumPageSize: request.MaximumPageSize,
		NextPageToken:   request.NextPageToken,
	}

	// Set start time filter if provided
	if request.StartTimeFilter != nil {
		cadenceRequest.StartTimeFilter = &shared.StartTimeFilter{
			EarliestTime: request.StartTimeFilter.EarliestTime,
			LatestTime:   request.StartTimeFilter.LatestTime,
		}
	}

	// Set execution filter if provided
	if request.ExecutionFilter != nil {
		cadenceRequest.ExecutionFilter = &shared.WorkflowExecutionFilter{
			WorkflowId: &request.ExecutionFilter.WorkflowID,
			RunId:      &request.ExecutionFilter.RunID,
		}
	}

	response, err := c.Client.ListOpenWorkflow(ctx, cadenceRequest)
	if err != nil {
		return nil, err
	}

	// Convert Temporal response to our interface format
	executionsInfo := make([]clientInterface.WorkflowExecutionInfo, 0, len(response.Executions))
	for _, exec := range response.Executions {
		executionsInfo = append(executionsInfo, clientInterface.WorkflowExecutionInfo{
			Execution: &clientInterface.WorkflowExecution{
				ID:    exec.Execution.GetWorkflowId(),
				RunID: exec.Execution.GetRunId(),
			},
			ExecutionTime: time.Unix(0, *exec.ExecutionTime),
			Status:        mapCadenceStatusToInterface(exec.CloseStatus),
		})
	}

	return &clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions:    executionsInfo,
		NextPageToken: response.NextPageToken,
	}, nil
}

func (c *CadenceClient) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	return c.Client.TerminateWorkflow(ctx, workflowID, runID, reason, nil)
}

func (c *CadenceClient) ResetWorkflow(ctx context.Context, options clientInterface.ResetWorkflowOptions) (*clientInterface.WorkflowExecution, error) {
	resetRequest := &shared.ResetWorkflowExecutionRequest{
		Domain: &c.Domain,
		WorkflowExecution: &shared.WorkflowExecution{
			WorkflowId: &options.WorkflowID,
			RunId:      &options.RunID,
		},
		Reason:                &options.Reason,
		DecisionFinishEventId: &options.EventID,
	}

	if options.RequestID != "" {
		resetRequest.RequestId = &options.RequestID
	}

	response, err := c.Client.ResetWorkflow(ctx, resetRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to reset workflow: %w", err)
	}

	return &clientInterface.WorkflowExecution{
		ID:    options.WorkflowID,
		RunID: response.GetRunId(),
	}, nil
}

func (c *CadenceClient) GetWorkflowExecutionHistory(ctx context.Context, workflowID string, runID string, pageToken []byte, pageSize int32) (*clientInterface.WorkflowHistory, error) {
	// Use iterator-based API for getting history
	iter := c.Client.GetWorkflowHistory(ctx, workflowID, runID, false, shared.HistoryEventFilterTypeAllEvent)

	// Convert Cadence history events to our interface format
	events := make([]clientInterface.HistoryEvent, 0)
	var collectedEvents int32 = 0

	for iter.HasNext() && (pageSize == 0 || collectedEvents < pageSize) {
		event, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to get next history event: %w", err)
		}

		historyEvent := clientInterface.HistoryEvent{
			EventID:   event.GetEventId(),
			EventType: event.GetEventType().String(),
			EventTime: time.Unix(0, event.GetTimestamp()),
			Details:   make(map[string]interface{}),
		}

		// Add relevant event details based on event type
		// Currently handles: DecisionTaskCompleted, ActivityTaskScheduled,
		// ActivityTaskCompleted, ActivityTaskFailed, WorkflowExecutionStarted
		switch event.GetEventType() {
		case shared.EventTypeDecisionTaskCompleted:
			if attr := event.DecisionTaskCompletedEventAttributes; attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
			}
		case shared.EventTypeActivityTaskScheduled:
			if attr := event.ActivityTaskScheduledEventAttributes; attr != nil {
				historyEvent.Details["activity_id"] = attr.GetActivityId()
				historyEvent.Details["activity_type"] = attr.GetActivityType().GetName()
			}
		case shared.EventTypeActivityTaskCompleted:
			if attr := event.ActivityTaskCompletedEventAttributes; attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
				// Note: Cadence ActivityTaskCompletedEventAttributes doesn't directly contain activity_id
				// The activity_id is typically found in the corresponding ActivityTaskScheduled event
			}
		case shared.EventTypeActivityTaskFailed:
			if attr := event.ActivityTaskFailedEventAttributes; attr != nil {
				historyEvent.Details["reason"] = attr.GetReason()
				historyEvent.Details["details"] = attr.GetDetails()
				historyEvent.Details["identity"] = attr.GetIdentity()
			}
		case shared.EventTypeWorkflowExecutionStarted:
			if attr := event.WorkflowExecutionStartedEventAttributes; attr != nil {
				historyEvent.Details["workflow_type"] = attr.GetWorkflowType().GetName()
				historyEvent.Details["task_list"] = attr.GetTaskList().GetName()
			}
		}

		events = append(events, historyEvent)
		collectedEvents++
	}

	// Note: Iterator-based API doesn't provide page tokens, so we return empty token
	return &clientInterface.WorkflowHistory{
		Events:        events,
		NextPageToken: pageToken,
	}, nil
}

// Event type abstraction methods for Cadence
func (c *CadenceClient) GetActivityTaskScheduledEventType() string {
	return shared.EventTypeActivityTaskScheduled.String()
}

func (c *CadenceClient) GetActivityTaskCompletedEventType() string {
	return shared.EventTypeActivityTaskCompleted.String()
}

func (c *CadenceClient) GetDecisionTaskCompletedEventType() string {
	return shared.EventTypeDecisionTaskCompleted.String()
}
