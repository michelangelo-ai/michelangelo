package triggerrun

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Runner defines the interface for trigger execution engines.
//
// This interface abstracts workflow lifecycle operations for different trigger types
// (cron, backfill, interval, batch rerun). Each Runner implementation manages workflow
// execution through Cadence or Temporal, providing status tracking and termination support.
//
// All methods return a TriggerRunStatus containing the execution state and metadata
// including workflow ID, run ID, and workflow UI URL.
//
// Implementations:
//   - cronTrigger: Manages recurring workflows with cron schedules
//   - backfillTrigger: Manages one-time backfill workflows
//   - intervalTrigger: Manages interval-based workflows (planned)
//   - batchRerunTrigger: Manages batch rerun workflows (planned)
type Runner interface {
	// Run starts workflow execution for a trigger run.
	//
	// This method initiates a workflow using the workflow client, configures execution
	// parameters (task list, timeouts, schedules), and returns the initial status.
	//
	// For recurring triggers (cron/interval), this starts a long-running workflow that
	// spawns child workflows on schedule. For one-time triggers (backfill), this starts
	// a single workflow execution.
	//
	// Returns TriggerRunStatus with State=RUNNING on success or State=FAILED on error.
	Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)

	// Kill terminates an active trigger run workflow.
	//
	// This method stops workflow execution by calling TerminateWorkflow on the workflow
	// client. For recurring triggers, this stops future scheduled executions.
	//
	// Returns TriggerRunStatus with State=KILLED on success. If the workflow is already
	// terminated or not running, returns KILLED without error.
	Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)

	// GetStatus retrieves the current execution status of a trigger run.
	//
	// This method queries the workflow engine for execution status and maps workflow
	// states to TriggerRunStatus states:
	//  - Running → RUNNING
	//  - Completed → SUCCEEDED
	//  - Failed/TimedOut → FAILED
	//  - Terminated/Canceled → KILLED
	//
	// For recurring triggers, this checks if an open workflow execution exists.
	// For one-time triggers, this describes the specific workflow execution.
	GetStatus(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)
}
