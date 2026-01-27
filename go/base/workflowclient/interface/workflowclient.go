package clientinterface

import (
	"context"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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

// ScheduledWorkflowOptions defines options for starting a scheduled workflow
type ScheduledWorkflowOptions struct {
	TriggerRun                      *v2pb.TriggerRun
	WorkflowType                    string
	TaskQueue                       string
	Args                            []interface{}
	ExecutionStartToCloseTimeout    time.Duration
	DecisionTaskStartToCloseTimeout time.Duration
}

// ScheduleStatus represents the status of a scheduled workflow
type ScheduleStatus struct {
	State        string
	ErrorMessage string
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

	// Scheduling methods
	// StartScheduledWorkflow starts a scheduled workflow
	StartScheduledWorkflow(ctx context.Context, options ScheduledWorkflowOptions) (*WorkflowExecution, error)
	// SupportsSchedules returns true if this client supports scheduled workflows
	SupportsSchedules() bool
	// StopScheduledWorkflow stops a scheduled workflow by schedule ID
	StopScheduledWorkflow(ctx context.Context, scheduleID string) error
	// GetScheduleStatus gets the status of a scheduled workflow
	GetScheduleStatus(ctx context.Context, scheduleID string) (*ScheduleStatus, error)
}
