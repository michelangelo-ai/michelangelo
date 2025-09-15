package temporalclient

import (
	"context"
	"encoding/base64"
	"fmt"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	temporalClient "go.temporal.io/sdk/client"
)

type TemporalClient struct {
	Client   temporalClient.Client
	Provider string
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

	temporalStatus := workflowExecutionInfo.Status

	switch temporalStatus {
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusUnSpecified,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusRunning,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusCompleted,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_FAILED:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusFailed,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusCanceled,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusTerminated,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusContinuedAsNew,
		}, nil
	case temporalEnumsV1.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusTimedOut,
		}, nil
	}
	return nil, fmt.Errorf("unknown workflow execution status: %s", temporalStatus)
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
