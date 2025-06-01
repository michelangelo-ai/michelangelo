package cadenceclient

import (
	"context"
	"fmt"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"go.uber.org/cadence/.gen/go/shared"
	cadenceClient "go.uber.org/cadence/client"
)

type CadenceClient struct {
	Client   cadenceClient.Client
	Provider string
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

	if !cadenceWorkflowExecutionInfo.IsSetCloseStatus() {
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusRunning,
		}, nil
	}

	closeStatus := cadenceWorkflowExecutionInfo.GetCloseStatus()
	switch closeStatus {
	case shared.WorkflowExecutionCloseStatusCompleted:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusCompleted,
		}, nil
	case shared.WorkflowExecutionCloseStatusFailed:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusFailed,
		}, nil
	case shared.WorkflowExecutionCloseStatusCanceled:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusCanceled,
		}, nil
	case shared.WorkflowExecutionCloseStatusTerminated:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusTerminated,
		}, nil
	case shared.WorkflowExecutionCloseStatusContinuedAsNew:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusContinuedAsNew,
		}, nil
	case shared.WorkflowExecutionCloseStatusTimedOut:
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusTimedOut,
		}, nil
	}

	return nil, fmt.Errorf("unknown workflow execution status: %s", closeStatus)
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
