package common

import (
	"fmt"
	"time"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// ReasonMetadataWriteFailed is reported when an actor cannot persist its progress.
const ReasonMetadataWriteFailed = "MetadataWriteFailed"

// ReportTransient reports an error that a retry may clear, such as an unreachable cluster or
// a failed ConfigMap or HTTPRoute update. A FALSE condition from an actor's Run ends the
// rollout, so until the actor has been working for longer than timeout the error is reported
// as UNKNOWN and the engine retries on the next reconcile. Once the budget is spent the same
// error becomes the terminal FALSE under timeoutReason, so a persistent outage still ends the
// rollout instead of retrying forever. A zero timeout never expires.
//
// The progress, with its start time, is written back to the condition so the next reconcile
// keeps counting from the first failure; callers start the clock before their first attempt.
func ReportTransient(condition *apipb.Condition, progress RolloutProgress, now time.Time, timeout time.Duration, timeoutReason, reason, message string) *apipb.Condition {
	if progress.TimedOut(now, timeout) {
		return conditionsutil.GenerateFalseCondition(condition, timeoutReason,
			fmt.Sprintf("%s: %s (still failing after %s)", reason, message, timeout))
	}
	if err := WriteRolloutProgress(condition, progress); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, ReasonMetadataWriteFailed, err.Error())
	}
	return conditionsutil.GenerateUnknownCondition(condition, reason, message)
}

// StartClock records the start time in progress if it is not set yet and returns it. It
// does not write the progress to a condition; ReportTransient and the actor's own writes do.
func StartClock(progress RolloutProgress, now time.Time) RolloutProgress {
	if !progress.Started() {
		progress.StartedAt = now.Unix()
	}
	return progress
}
