package pipelinerunutils

import (
	"errors"
	"fmt"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Actors signal failure to the condition engine in one of two ways.
//
// A non-retryable failure is returned as an error. The engine treats any error
// as critical and terminal, and the controller marks the run FAILED. Because the
// engine returns before persisting the condition, terminal paths should record
// their explanation on the step (step.Message) rather than on the condition.
//
// A retryable failure is returned as an UNKNOWN condition with a nil error. The
// engine leaves the resource unsatisfied and requeues it, so the run keeps its
// current state instead of failing. This matters because a pipeline run must not
// be failed by an infrastructure blip: a genuine pipeline failure reaches the
// controller as a workflow execution status (Failed, TimedOut), never as a
// client error, so an error talking to the workflow service is by construction
// transport-level and worth retrying.

const (
	// StartupRetryDeadline bounds how long a transient failure may keep a run
	// requeueing before it is given up on as terminal. It applies to the paths
	// that run before the workflow exists - fetching the project, resolving the
	// task list, starting the workflow - where nothing else is tracking the run.
	//
	// Reads against an already-running workflow are deliberately unbounded (see
	// NoRetryDeadline): the workflow is the source of truth for those runs, and a
	// long workflow-service outage must not fail pipelines that are still healthy.
	//
	// TODO: make this configurable alongside the condition engine's requeue
	// period, which is likewise hardcoded today.
	StartupRetryDeadline = 30 * time.Minute

	// NoRetryDeadline disables the deadline, retrying for as long as the failure
	// persists.
	NoRetryDeadline time.Duration = 0
)

// Reasons for transient conditions. These are compared across reconciles to
// decide whether a failure is the same one we saw last time, so they must be
// stable identifiers and never interpolate error text.
const (
	ReasonManualRetryFailed         = "ManualRetryFailed"
	ReasonWorkflowTerminationFailed = "WorkflowTerminationFailed"
	ReasonProjectFetchFailed        = "ProjectFetchFailed"
	ReasonWorkflowStartFailed       = "WorkflowStartFailed"
	ReasonWorkflowStatusUnavailable = "WorkflowStatusUnavailable"
	ReasonPipelineFetchFailed       = "PipelineFetchFailed"
)

// TransientCondition builds the UNKNOWN condition an actor returns for a
// retryable failure, or an error once the failure has outlasted the deadline.
//
// When the same failure repeats, the previously persisted condition is returned
// unchanged. That is deliberate: the controller only writes status when it
// differs from what it read, and a status write feeds the controller's own watch,
// which would re-reconcile immediately and defeat the engine's requeue delay. A
// condition carrying a counter or a varying message would therefore spin rather
// than back off, so elapsed time is measured from the timestamp on the first
// occurrence instead.
//
// Pass NoRetryDeadline to retry indefinitely.
func TransientCondition(
	pipelineRun *v2.PipelineRun,
	conditionType string,
	reason string,
	err error,
	deadline time.Duration,
) (*apipb.Condition, error) {
	now := time.Now()
	existing := conditionUtils.GetCondition(conditionType, pipelineRun.Status.Conditions)

	if existing != nil && existing.Status == apipb.CONDITION_STATUS_UNKNOWN && existing.Reason == reason {
		if deadline > NoRetryDeadline && existing.LastUpdatedTimestamp > 0 {
			if elapsed := now.Sub(time.Unix(existing.LastUpdatedTimestamp, 0)); elapsed > deadline {
				return nil, fmt.Errorf("%s: still failing after %s, giving up: %w",
					reason, elapsed.Truncate(time.Second), err)
			}
		}
		return existing, nil
	}

	return &apipb.Condition{
		Type:                 conditionType,
		Status:               apipb.CONDITION_STATUS_UNKNOWN,
		Reason:               reason,
		Message:              err.Error(),
		LastUpdatedTimestamp: now.Unix(),
	}, nil
}

// TerminalError marks a failure that retrying cannot resolve. Helpers that mix
// retryable and non-retryable failures wrap the latter so their callers can tell
// the two apart without inspecting error strings.
type TerminalError struct{ Err error }

func (e *TerminalError) Error() string { return e.Err.Error() }

func (e *TerminalError) Unwrap() error { return e.Err }

// Terminalf builds a TerminalError from a format string.
func Terminalf(format string, args ...interface{}) error {
	return &TerminalError{Err: fmt.Errorf(format, args...)}
}

// IsTerminal reports whether err was marked non-retryable with Terminalf.
func IsTerminal(err error) bool {
	var terminal *TerminalError
	return errors.As(err, &terminal)
}

// RetryableAPIError reports whether an error from a Kubernetes or gRPC read is
// worth retrying. A NotFound means the referenced resource genuinely does not
// exist, and retrying cannot conjure it.
func RetryableAPIError(err error) bool {
	return !utils.IsNotFoundError(err)
}
