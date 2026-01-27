package temporalclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	enumspb "go.temporal.io/api/enums/v1"
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
		Overlap:        temporalEnumsV1.SCHEDULE_OVERLAP_POLICY_SKIP,
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

// StartScheduledWorkflow creates an actual Temporal Schedule using the cron expression from TriggerRun
func (c *TemporalClient) StartScheduledWorkflow(ctx context.Context, options clientInterface.ScheduledWorkflowOptions) (*clientInterface.WorkflowExecution, error) {
	scheduleID := fmt.Sprintf("%s-%s", options.TriggerRun.Namespace, options.TriggerRun.Name)

	// Extract cron expression from TriggerRun
	cronSchedule := options.TriggerRun.Spec.Trigger.GetCronSchedule()
	if cronSchedule == nil {
		// Fallback to regular workflow execution for non-cron triggers
		workflowOptions := temporalClient.StartWorkflowOptions{
			ID:                       scheduleID,
			TaskQueue:                options.TaskQueue,
			WorkflowExecutionTimeout: options.ExecutionStartToCloseTimeout,
			WorkflowTaskTimeout:      options.DecisionTaskStartToCloseTimeout,
		}

		run, err := c.Client.ExecuteWorkflow(ctx, workflowOptions, options.WorkflowType, options.Args...)
		if err != nil {
			return nil, fmt.Errorf("failed to start regular workflow: %w", err)
		}

		return &clientInterface.WorkflowExecution{
			ID:    run.GetID(),
			RunID: run.GetRunID(),
		}, nil
	}

	cronExpression := cronSchedule.GetCron()
	if cronExpression == "" {
		return nil, fmt.Errorf("cron expression is empty")
	}

	// Create Temporal Schedule
	scheduleClient := c.Client.ScheduleClient()

	schedule := temporalClient.ScheduleSpec{
		CronExpressions: []string{cronExpression},
	}

	action := &temporalClient.ScheduleWorkflowAction{
		ID:                       scheduleID + "-workflow",
		Workflow:                 options.WorkflowType,
		Args:                     options.Args,
		TaskQueue:                options.TaskQueue,
		WorkflowExecutionTimeout: options.ExecutionStartToCloseTimeout,
		WorkflowTaskTimeout:      options.DecisionTaskStartToCloseTimeout,
	}

	scheduleOptions := temporalClient.ScheduleOptions{
		ID:      scheduleID,
		Spec:    schedule,
		Action:  action,
		Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, // Prevent overlapping runs
	}

	scheduleHandle, err := scheduleClient.Create(ctx, scheduleOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal schedule: %w", err)
	}

	// Return a mock execution since schedules don't have a single workflow execution
	return &clientInterface.WorkflowExecution{
		ID:    scheduleHandle.GetID(),
		RunID: "schedule-" + scheduleHandle.GetID(), // Fake RunID for schedule
	}, nil
}

// SupportsSchedules returns true for Temporal client since it supports schedules natively
func (c *TemporalClient) SupportsSchedules() bool {
	return true
}

// StopScheduledWorkflow stops a Temporal Schedule by deleting it
func (c *TemporalClient) StopScheduledWorkflow(ctx context.Context, scheduleID string) error {
	scheduleClient := c.Client.ScheduleClient()
	scheduleHandle := scheduleClient.GetHandle(ctx, scheduleID)

	// Delete the schedule to stop it
	err := scheduleHandle.Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete temporal schedule: %w", err)
	}

	return nil
}

// GetScheduleStatus gets the status of a Temporal Schedule
func (c *TemporalClient) GetScheduleStatus(ctx context.Context, scheduleID string) (*clientInterface.ScheduleStatus, error) {
	scheduleClient := c.Client.ScheduleClient()
	scheduleHandle := scheduleClient.GetHandle(ctx, scheduleID)

	// Get schedule info
	scheduleInfo, err := scheduleHandle.Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get temporal schedule info: %w", err)
	}

	var state string
	var errorMessage string

	// Check if schedule is paused
	if scheduleInfo.Schedule.State != nil && scheduleInfo.Schedule.State.Paused {
		state = "PAUSED"
	} else {
		// Schedule is active
		state = "RUNNING"
	}

	// Check for recent action results to see if there are failures
	if len(scheduleInfo.Info.RecentActions) > 0 {
		recentAction := scheduleInfo.Info.RecentActions[0]
		if recentAction.StartWorkflowResult == nil {
			state = "FAILED"
			errorMessage = "Failed to start workflow"
		}
	}

	return &clientInterface.ScheduleStatus{
		State:        state,
		ErrorMessage: errorMessage,
	}, nil
}
