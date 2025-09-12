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
			Status: mapCadenceStatusToInterface(exec.CloseStatus),
		})
	}

	return &clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions:    executionsInfo,
		NextPageToken: response.NextPageToken,
	}, nil
}