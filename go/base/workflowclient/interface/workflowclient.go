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
	CronSchedule                    string
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
	Status        WorkflowExecutionStatus
	Execution     *WorkflowExecution
	ExecutionTime time.Time
}

type WorkflowExecution struct {
	ID    string
	RunID string
}

type ResetWorkflowOptions struct {
	WorkflowID string
	RunID      string
	EventID    int64
	Reason     string
	RequestID  string
}

type HistoryEvent struct {
	EventID   int64
	EventType string
	EventTime time.Time
	Details   map[string]interface{}
}

type WorkflowHistory struct {
	Events        []HistoryEvent
	NextPageToken []byte
}

type ExecutionFilter struct {
	WorkflowID string
	RunID      string
}

type StartTimeFilter struct {
	EarliestTime *int64
	LatestTime   *int64
}

type ListOpenWorkflowExecutionsRequest struct {
	Domain          string
	MaximumPageSize *int32
	NextPageToken   []byte
	StartTimeFilter *StartTimeFilter
	ExecutionFilter *ExecutionFilter
}

type ListOpenWorkflowExecutionsResponse struct {
	Executions    []WorkflowExecutionInfo
	NextPageToken []byte
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
	// GetDomain gets the domain of the client
	GetDomain() string
	// ListOpenWorkflow lists the open workflows with the given request
	ListOpenWorkflow(ctx context.Context, request ListOpenWorkflowExecutionsRequest) (*ListOpenWorkflowExecutionsResponse, error)
	// TerminateWorkflow terminates a workflow
	TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error
	// ResetWorkflow resets a workflow execution to a specific event ID
	ResetWorkflow(ctx context.Context, options ResetWorkflowOptions) (*WorkflowExecution, error)
	// GetWorkflowExecutionHistory gets the execution history of a workflow
	GetWorkflowExecutionHistory(ctx context.Context, workflowID string, runID string, pageToken []byte, pageSize int32) (*WorkflowHistory, error)
	// GetActivityTaskScheduledEventType returns the engine-specific event type string for ActivityTaskScheduled
	GetActivityTaskScheduledEventType() string
	// GetActivityTaskCompletedEventType returns the engine-specific event type string for ActivityTaskCompleted
	GetActivityTaskCompletedEventType() string
	// GetDecisionTaskCompletedEventType returns the engine-specific event type string for DecisionTaskCompleted
	GetDecisionTaskCompletedEventType() string
	// PauseSchedule pauses a workflow schedule by schedule ID
	PauseSchedule(ctx context.Context, scheduleID string) error
	// UnpauseSchedule unpauses a workflow schedule by schedule ID
	UnpauseSchedule(ctx context.Context, scheduleID string) error
	// DeleteSchedule deletes a workflow schedule by schedule ID
	DeleteSchedule(ctx context.Context, scheduleID string) error
}
