package triggerrun

import (
	"fmt"
	"time"

	"github.com/robfig/cron"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// effectiveCatchUpPolicy is the catch-up policy a spec asks for once the boolean
// shorthand is folded in: an explicit trigger.catch_up_policy wins; otherwise
// catchup=true reads as ALL and everything else as SKIP. The API hook rejects a
// disagreeing pair, so by the time a spec reaches the controller the two agree.
func effectiveCatchUpPolicy(spec *v2pb.TriggerRunSpec) v2pb.CatchUpPolicy {
	if policy := spec.GetTrigger().GetCatchUpPolicy(); policy != v2pb.CATCH_UP_POLICY_UNSPECIFIED {
		return policy
	}
	if spec.GetCatchup() {
		return v2pb.CATCH_UP_POLICY_ALL
	}
	return v2pb.CATCH_UP_POLICY_SKIP
}

// isLegacyBackfillShape reports whether a spec has the shape that, before
// catch_up_policy existed, selected the one-shot backfill runner: a closed window
// and no catch-up of any kind. Any explicit policy means a recurring schedule with
// a window, so adding one never moves an object between runners by accident.
func isLegacyBackfillShape(spec *v2pb.TriggerRunSpec) bool {
	return spec.StartTimestamp != nil && spec.EndTimestamp != nil &&
		!spec.GetCatchup() &&
		spec.GetTrigger().GetCatchUpPolicy() == v2pb.CATCH_UP_POLICY_UNSPECIFIED
}

// overlapPolicyOption maps the API enum onto the engine-neutral workflow-client enum.
// Unspecified maps to the client's zero value, which every adapter treats as skip-new.
func overlapPolicyOption(policy v2pb.OverlapPolicy) (clientInterface.OverlapPolicy, error) {
	switch policy {
	case v2pb.OVERLAP_POLICY_UNSPECIFIED, v2pb.OVERLAP_POLICY_SKIP:
		return clientInterface.OverlapPolicySkipNew, nil
	case v2pb.OVERLAP_POLICY_BUFFER:
		return clientInterface.OverlapPolicyBuffer, nil
	case v2pb.OVERLAP_POLICY_ALLOW:
		return clientInterface.OverlapPolicyConcurrent, nil
	case v2pb.OVERLAP_POLICY_REPLACE:
		return clientInterface.OverlapPolicyCancelPrevious, nil
	default:
		return clientInterface.OverlapPolicyUnspecified, fmt.Errorf("unknown overlap_policy %v", policy)
	}
}

// catchUpPolicyOption maps the API enum onto the engine-neutral workflow-client enum.
func catchUpPolicyOption(policy v2pb.CatchUpPolicy) (clientInterface.CatchUpPolicy, error) {
	switch policy {
	case v2pb.CATCH_UP_POLICY_UNSPECIFIED, v2pb.CATCH_UP_POLICY_SKIP:
		return clientInterface.CatchUpPolicySkip, nil
	case v2pb.CATCH_UP_POLICY_LATEST:
		return clientInterface.CatchUpPolicyOne, nil
	case v2pb.CATCH_UP_POLICY_ALL:
		return clientInterface.CatchUpPolicyAll, nil
	default:
		return clientInterface.CatchUpPolicyUnspecified, fmt.Errorf("unknown catch_up_policy %v", policy)
	}
}

// lastCronOccurrenceBefore returns the latest occurrence of cronExpr in [from, to),
// or the zero time when there is none. It walks forward from `from`, so the cost is
// proportional to the number of occurrences in the range; callers pass a window the
// API already bounded.
func lastCronOccurrenceBefore(cronExpr string, from, to time.Time) (time.Time, error) {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	var last time.Time
	// Next is strictly after its argument at second granularity; step back one second
	// so an occurrence that falls exactly on `from` is considered.
	for t := schedule.Next(from.Add(-time.Second)); !t.IsZero() && t.Before(to); t = schedule.Next(t) {
		last = t
	}
	return last, nil
}
