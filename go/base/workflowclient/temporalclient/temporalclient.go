package temporalclient

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	filterV1 "go.temporal.io/api/filter/v1"
	temporalClient "go.temporal.io/sdk/client"
	workflowserviceV1 "go.temporal.io/api/workflowservice/v1"
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
	startWorkflowOptions := temporalClient.StartWorkflowOptions{
		ID:                       options.ID,
		TaskQueue:                options.TaskList,
		WorkflowExecutionTimeout: options.ExecutionStartToCloseTimeout,
		WorkflowTaskTimeout:      options.DecisionTaskStartToCloseTimeout,
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
		Namespace:       request.Domain,
		MaximumPageSize: *request.MaximumPageSize,
		NextPageToken:   request.NextPageToken,
	}

	// Set start time filter if provided
	if request.StartTimeFilter != nil {
		temporalRequest.StartTimeFilter = &filterV1.StartTimeFilter{
			EarliestTime: timestamppb.New(time.Unix(*request.StartTimeFilter.EarliestTime, 0)),
			LatestTime:   timestamppb.New(time.Unix(*request.StartTimeFilter.LatestTime, 0)),
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
			Status: mapTemporalStatusToInterface(exec.Status),
		})
	}

	return &clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions:    executionsInfo,
		NextPageToken: response.NextPageToken,
	}, nil
}
