package triggerrun

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Runner interface to be implemented for different triggering engines, initial phase only Cadence is supported
// Each method return a PipelineRunStatus, which contains execution state and metadata, e.g. workflow id, url, etc.
type Runner interface {
	// Run start a trigger run
	Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)

	// Kill terminate a trigger run
	Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)

	// GetStatus get status of a trigger run
	GetStatus(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error)
}
