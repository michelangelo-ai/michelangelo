package clientinterface

import (
	"context"
	"time"
)

type StartWorkflowOptions struct {
	ID                              string
	TaskList                        string
	ExecutionStartToCloseTimeout    time.Duration
	DecisionTaskStartToCloseTimeout time.Duration
}

type WorkflowExecutionStatus int32

const (
	WorkflowExecutionStatusUnSpecified    WorkflowExecutionStatus = 0
	WorkflowExecutionStatusRunning        WorkflowExecutionStatus = 1
	WorkflowExecutionStatusCompleted      WorkflowExecutionStatus = 2
	WorkflowExecutionStatusFailed         WorkflowExecutionStatus = 3
	WorkflowExecutionStatusCanceled       WorkflowExecutionStatus = 4
	WorkflowExecutionStatusTerminated     WorkflowExecutionStatus = 5
	WorkflowExecutionStatusContinuedAsNew WorkflowExecutionStatus = 6
	WorkflowExecutionStatusTimedOut       WorkflowExecutionStatus = 7
)

type WorkflowExecutionInfo struct {
	Status WorkflowExecutionStatus
}

type WorkflowExecution struct {
	ID    string
	RunID string
}

type WorkflowClient interface {
	// StartWorkflow starts a new workflow
	StartWorkflow(ctx context.Context, options StartWorkflowOptions, workflowName string, args ...interface{}) (*WorkflowExecution, error)
	// GetWorkflowExecutionInfo gets the execution info of a workflow
	GetWorkflowExecutionInfo(ctx context.Context, workflowID string, runID string) (*WorkflowExecutionInfo, error)
	// CancelWorkflow cancels a workflow
	CancelWorkflow(ctx context.Context, workflowID string, runID string, reason string) error
	// QueryWorkflow queries a workflow
	QueryWorkflow(ctx context.Context, workflowID string, runID string, queryHandlerKey string, queryResult any) error
	// GetProvider gets the provider of the client
	GetProvider() string
}
