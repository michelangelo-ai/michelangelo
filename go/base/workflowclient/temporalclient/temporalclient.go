package temporalclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	commonV1 "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	filterV1 "go.temporal.io/api/filter/v1"
	workflowserviceV1 "go.temporal.io/api/workflowservice/v1"
	temporalClient "go.temporal.io/sdk/client"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mapTemporalStatusToInterface maps Temporal workflow status to our interface status
func mapTemporalStatusToInterface(status temporalEnumsV1.WorkflowExecutionStatus) clientInterface.WorkflowExecutionStatus {
	switch status {
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return clientInterface.WorkflowExecutionStatusRunning
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return clientInterface.WorkflowExecutionStatusCompleted
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_FAILED:
		return clientInterface.WorkflowExecutionStatusFailed
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return clientInterface.WorkflowExecutionStatusCanceled
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return clientInterface.WorkflowExecutionStatusTerminated
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return clientInterface.WorkflowExecutionStatusContinuedAsNew
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return clientInterface.WorkflowExecutionStatusTimedOut
	default:
		return clientInterface.WorkflowExecutionStatusUnSpecified
	}
}

type TemporalClient struct {
	Client   temporalClient.Client
	Provider string
	Domain   string
}

// ensure TemporalClient implements clientInterface.WorkflowClient
var _ clientInterface.WorkflowClient = &TemporalClient{}

// StartWorkflow starts a new workflow
func (c *TemporalClient) StartWorkflow(ctx context.Context, options clientInterface.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterface.WorkflowExecution, error) {
	// If CronSchedule is provided, create a Temporal Schedule instead of a cron workflow
	if options.CronSchedule != "" {
		return c.createScheduleForCron(ctx, options, workflowName, args...)
	}

	startWorkflowOptions := temporalClient.StartWorkflowOptions{
		ID:                       options.ID,
		TaskQueue:                options.TaskList,
		WorkflowExecutionTimeout: options.ExecutionStartToCloseTimeout,
		WorkflowTaskTimeout:      options.DecisionTaskStartToCloseTimeout,
		// No CronSchedule - this is a regular workflow
	}
	// This is a workaround for Grab Temporal demo
	_args := make([]any, len(args))
	for i, a := range args {
		if i == 0 {
			arg0, ok := a.([]uint8)
			if !ok {
				_args[i] = a
			} else {
				_args[i] = base64.StdEncoding.EncodeToString(arg0)
			}
		} else {
			_args[i] = a
		}
	}
	workflowExecution, err := c.Client.ExecuteWorkflow(ctx, startWorkflowOptions, workflowName, _args...)
	if err != nil {
		return nil, err
	}
	return &clientInterface.WorkflowExecution{
		ID:    workflowExecution.GetID(),
		RunID: workflowExecution.GetRunID(),
	}, nil
}

// createScheduleForCron creates a Temporal Schedule when a cron expression is provided in StartWorkflow
func (c *TemporalClient) createScheduleForCron(ctx context.Context, options clientInterface.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterface.WorkflowExecution, error) {
	// Generate a schedule ID based on the workflow ID
	scheduleID := options.ID + "-schedule"

	// Schedule should always fire when cron time is met
	// The trigger workflow itself will handle maxConcurrency logic for pipeline runs
	overlapPolicy := temporalEnumsV1.SCHEDULE_OVERLAP_POLICY_SKIP

	// Log the configuration for debugging
	fmt.Printf("[TemporalClient] Creating schedule %s with overlapPolicy=%v (trigger workflow handles maxConcurrency)\n",
		scheduleID, overlapPolicy)

	// Check if schedule already exists
	scheduleHandle := c.Client.ScheduleClient().GetHandle(ctx, scheduleID)
	if scheduleHandle != nil {
		_, err := scheduleHandle.Describe(ctx)
		if err == nil {
			// Schedule already exists, return success
			return &clientInterface.WorkflowExecution{
				ID:    scheduleID,
				RunID: "", // Schedules don't have runIDs
			}, nil
		}
	}

	// Create Temporal Schedule
	scheduleOptions := temporalClient.ScheduleOptions{
		ID: scheduleID,
		Spec: temporalClient.ScheduleSpec{
			CronExpressions: []string{options.CronSchedule},
		},
		Action: &temporalClient.ScheduleWorkflowAction{
			ID:        options.ID,
			Workflow:  workflowName,
			TaskQueue: options.TaskList,
			Args:      args,
		},
		Overlap:        overlapPolicy, // Use extracted policy based on maxConcurrency
		PauseOnFailure: false,
	}

	// Set workflow timeout if provided
	if options.ExecutionStartToCloseTimeout > 0 {
		scheduleOptions.Action.(*temporalClient.ScheduleWorkflowAction).WorkflowExecutionTimeout = options.ExecutionStartToCloseTimeout
	}

	// Create the schedule
	_, err := c.Client.ScheduleClient().Create(ctx, scheduleOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal schedule: %w", err)
	}

	return &clientInterface.WorkflowExecution{
		ID:    scheduleID,
		RunID: "", // Schedules don't have runIDs
	}, nil
}

// GetWorkflowExecutionInfo gets the execution info of a workflow
func (c *TemporalClient) GetWorkflowExecutionInfo(ctx context.Context, workflowID string, runID string) (*clientInterface.WorkflowExecutionInfo, error) {
	describeWorkflowResponse, err := c.Client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	workflowExecutionInfo := describeWorkflowResponse.WorkflowExecutionInfo

	if workflowExecutionInfo == nil {
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusUnSpecified,
		}, nil
	}

	return &clientInterface.WorkflowExecutionInfo{
		Status: mapTemporalStatusToInterface(workflowExecutionInfo.Status),
	}, nil
}

// QueryWorkflow queries a workflow
func (c *TemporalClient) QueryWorkflow(ctx context.Context, workflowID string, runID string, queryHandler string, queryResult any) error {
	request := temporalClient.QueryWorkflowWithOptionsRequest{
		WorkflowID: workflowID,
		RunID:      runID,
		QueryType:  queryHandler,
	}
	response, err := c.Client.QueryWorkflowWithOptions(ctx, &request)
	if err != nil {
		return fmt.Errorf("failed to query workflow: %w", err)
	}

	if response == nil || response.QueryResult == nil {
		return fmt.Errorf("queryResult is nil")
	}

	// decode the query result to the queryResult
	if err = response.QueryResult.Get(&queryResult); err != nil {
		return fmt.Errorf("failed to decode query result: %w", err)
	}
	return nil
}

// CancelWorkflow cancels a workflow
func (c *TemporalClient) CancelWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	return c.Client.CancelWorkflow(ctx, workflowID, runID)
}

// GetProvider gets the provider of the client
func (c *TemporalClient) GetProvider() string {
	return c.Provider
}

func (c *TemporalClient) GetDomain() string {
	return c.Domain
}

func (c *TemporalClient) ListOpenWorkflow(ctx context.Context, request clientInterface.ListOpenWorkflowExecutionsRequest) (*clientInterface.ListOpenWorkflowExecutionsResponse, error) {
	temporalRequest := &workflowserviceV1.ListOpenWorkflowExecutionsRequest{
		Namespace:     request.Domain,
		NextPageToken: request.NextPageToken,
	}

	// Only set MaximumPageSize if provided, let Temporal use its default otherwise
	if request.MaximumPageSize != nil {
		temporalRequest.MaximumPageSize = *request.MaximumPageSize
	}

	// Set start time filter if provided
	if request.StartTimeFilter != nil {
		// Convert nanoseconds to time.Time for Temporal's timestamppb
		earliestTime := time.Unix(0, *request.StartTimeFilter.EarliestTime)
		latestTime := time.Unix(0, *request.StartTimeFilter.LatestTime)

		temporalRequest.StartTimeFilter = &filterV1.StartTimeFilter{
			EarliestTime: timestamppb.New(earliestTime),
			LatestTime:   timestamppb.New(latestTime),
		}
	}

	// Set execution filter if provided
	if request.ExecutionFilter != nil {
		temporalRequest.Filters = &workflowserviceV1.ListOpenWorkflowExecutionsRequest_ExecutionFilter{
			ExecutionFilter: &filterV1.WorkflowExecutionFilter{
				WorkflowId: request.ExecutionFilter.WorkflowID,
				RunId:      request.ExecutionFilter.RunID,
			},
		}
	}

	response, err := c.Client.ListOpenWorkflow(ctx, temporalRequest)
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
			ExecutionTime: exec.StartTime.AsTime(),
			Status:        mapTemporalStatusToInterface(exec.Status),
		})
	}

	return &clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions:    executionsInfo,
		NextPageToken: response.NextPageToken,
	}, nil
}

func (c *TemporalClient) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	return c.Client.TerminateWorkflow(ctx, workflowID, runID, reason)
}

func (c *TemporalClient) ResetWorkflow(ctx context.Context, options clientInterface.ResetWorkflowOptions) (*clientInterface.WorkflowExecution, error) {
	resetRequest := &workflowserviceV1.ResetWorkflowExecutionRequest{
		Namespace: c.Domain,
		WorkflowExecution: &commonV1.WorkflowExecution{
			WorkflowId: options.WorkflowID,
			RunId:      options.RunID,
		},
		Reason:                    options.Reason,
		WorkflowTaskFinishEventId: options.EventID,
	}

	if options.RequestID != "" {
		resetRequest.RequestId = options.RequestID
	}

	response, err := c.Client.ResetWorkflowExecution(ctx, resetRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to reset workflow: %w", err)
	}

	return &clientInterface.WorkflowExecution{
		ID:    options.WorkflowID,
		RunID: response.GetRunId(),
	}, nil
}

func (c *TemporalClient) GetWorkflowExecutionHistory(ctx context.Context, workflowID string, runID string, pageToken []byte, pageSize int32) (*clientInterface.WorkflowHistory, error) {
	// Use iterator-based API for getting history
	iter := c.Client.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)

	// Convert Temporal history events to our interface format
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
			EventTime: event.GetEventTime().AsTime(),
			Details:   make(map[string]interface{}),
		}

		// Add relevant event details based on event type
		// Currently handles: WorkflowTaskCompleted, ActivityTaskScheduled,
		// ActivityTaskCompleted, ActivityTaskFailed, WorkflowExecutionStarted
		switch event.GetEventType() {
		case temporalEnumsV1.EVENT_TYPE_WORKFLOW_TASK_COMPLETED:
			if attr := event.GetWorkflowTaskCompletedEventAttributes(); attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
			}
		case temporalEnumsV1.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
			if attr := event.GetActivityTaskScheduledEventAttributes(); attr != nil {
				historyEvent.Details["activity_id"] = attr.GetActivityId()
				historyEvent.Details["activity_type"] = attr.GetActivityType().GetName()
			}
		case temporalEnumsV1.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
			if attr := event.GetActivityTaskCompletedEventAttributes(); attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
				// Note: Temporal ActivityTaskCompletedEventAttributes doesn't directly contain activity_id
				// The activity_id is typically found in the corresponding ActivityTaskScheduled event
			}
		case temporalEnumsV1.EVENT_TYPE_ACTIVITY_TASK_FAILED:
			if attr := event.GetActivityTaskFailedEventAttributes(); attr != nil {
				if failure := attr.GetFailure(); failure != nil {
					historyEvent.Details["failure_message"] = failure.GetMessage()
					historyEvent.Details["failure_source"] = failure.GetSource()
				}
				historyEvent.Details["identity"] = attr.GetIdentity()
			}
		case temporalEnumsV1.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			if attr := event.GetWorkflowExecutionStartedEventAttributes(); attr != nil {
				historyEvent.Details["workflow_type"] = attr.GetWorkflowType().GetName()
				historyEvent.Details["task_queue"] = attr.GetTaskQueue().GetName()
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

// Event type abstraction methods for Temporal
func (c *TemporalClient) GetActivityTaskScheduledEventType() string {
	return temporalEnumsV1.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED.String()
}

func (c *TemporalClient) GetActivityTaskCompletedEventType() string {
	return temporalEnumsV1.EVENT_TYPE_ACTIVITY_TASK_COMPLETED.String()
}

func (c *TemporalClient) GetDecisionTaskCompletedEventType() string {
	// In Temporal, DecisionTask is called WorkflowTask
	return temporalEnumsV1.EVENT_TYPE_WORKFLOW_TASK_COMPLETED.String()
}

// PauseTrigger pauses the Temporal schedule associated with the given workflow ID.
func (c *TemporalClient) PauseTrigger(ctx context.Context, workflowID string) error {
	scheduleID := workflowID + "-schedule"
	handle := c.Client.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Pause(ctx, temporalClient.SchedulePauseOptions{
		Note: "paused by michelangelo",
	})
}

// UnpauseTrigger resumes the Temporal schedule associated with the given workflow ID.
func (c *TemporalClient) UnpauseTrigger(ctx context.Context, workflowID string) error {
	scheduleID := workflowID + "-schedule"
	handle := c.Client.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Unpause(ctx, temporalClient.ScheduleUnpauseOptions{
		Note: "unpaused by michelangelo",
	})
}

// DeleteTrigger deletes the Temporal schedule and terminates any running workflow execution.
func (c *TemporalClient) DeleteTrigger(ctx context.Context, workflowID string, runID string) error {
	scheduleID := workflowID + "-schedule"
	handle := c.Client.ScheduleClient().GetHandle(ctx, scheduleID)
	if err := handle.Delete(ctx); err != nil {
		return err
	}
	if runID == "" {
		return nil
	}
	return c.Client.TerminateWorkflow(ctx, workflowID, runID, "trigger killed")
}
