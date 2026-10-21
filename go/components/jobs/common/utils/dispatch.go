package utils

import (
	"fmt"
	"slices"
	"time"

	protoTypes "github.com/gogo/protobuf/types"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

const (
	// DispatchWindow bounds the retries of a job's creates on one target, and
	// the terminal-path cleanup after a create that may have reached it (RFC 0f).
	DispatchWindow = 10 * time.Minute

	dispatchBackoffBase = 5 * time.Second
	dispatchBackoffCap  = 60 * time.Second
)

// DispatchState is the dispatch record kept in a job's Launched condition
// metadata, so that retries, backoff and the windows survive restarts and
// leader failover (RFC 0f).
type DispatchState struct {
	// WindowStart is the time of the first failed create on the current target.
	WindowStart time.Time
	// Attempts counts the failed creates on the current target, attempts
	// skipped before dialing a target that is not Ready included.
	Attempts int
	// Ambiguous latches once a failed create on the current target may have
	// created the object.
	Ambiguous bool
	// NextAttemptAt is the earliest time of the next create.
	NextAttemptAt time.Time
	// Replacements counts the job's re-placements.
	Replacements int
	// ExcludedClusters lists the clusters the job was re-placed away from, in
	// order and without duplicates. Its next placement decision excludes them,
	// unless that leaves no candidate (RFC 0f).
	ExcludedClusters []string
	// CleanupStart is the time of the first terminal-path cleanup that needed
	// the owner-checked delete.
	CleanupStart time.Time
}

// DispatchBackoff returns the delay after the n-th failed attempt:
// 5s·2^(n−1), capped at 60s.
func DispatchBackoff(attempts int) time.Duration {
	backoff := dispatchBackoffBase
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff >= dispatchBackoffCap {
			return dispatchBackoffCap
		}
	}
	return backoff
}

// IsZero reports whether nothing has been recorded.
func (s DispatchState) IsZero() bool {
	return s.WindowStart.IsZero() && s.Attempts == 0 && !s.Ambiguous && s.NextAttemptAt.IsZero() &&
		s.Replacements == 0 && len(s.ExcludedClusters) == 0 && s.CleanupStart.IsZero()
}

// WithAttempt returns the state after a create that failed at now. The first
// failure opens the window, and ambiguity latches.
func (s DispatchState) WithAttempt(now time.Time, ambiguous bool) DispatchState {
	if s.WindowStart.IsZero() {
		s.WindowStart = now
	}
	s.Attempts++
	s.Ambiguous = s.Ambiguous || ambiguous
	s.NextAttemptAt = now.Add(DispatchBackoff(s.Attempts))
	return s
}

// WindowExpired reports whether the create window has run out at now.
func (s DispatchState) WindowExpired(now time.Time) bool {
	return !s.WindowStart.IsZero() && now.Sub(s.WindowStart) >= DispatchWindow
}

// WithReplacement returns the state after the job is re-placed at now, off
// the cluster left. The window on the old target is dropped, but the backoff
// is kept: the first create after the r-th re-placement waits
// min(5s·2^r, 60s), so a job pinned to a target that is not Ready cannot
// cycle hot. The cluster left joins the excluded clusters, unless it is
// there already.
func (s DispatchState) WithReplacement(now time.Time, left string) DispatchState {
	s.WindowStart = time.Time{}
	s.Attempts = 0
	s.Ambiguous = false
	s.Replacements++
	s.NextAttemptAt = now.Add(DispatchBackoff(s.Replacements + 1))
	if left != "" && !slices.Contains(s.ExcludedClusters, left) {
		// A copy, so that the state this one was derived from is unchanged.
		s.ExcludedClusters = append(slices.Clip(s.ExcludedClusters), left)
	}
	return s
}

// WithCleanupStart returns the state with the cleanup window opened at now,
// unless it is open already.
func (s DispatchState) WithCleanupStart(now time.Time) DispatchState {
	if s.CleanupStart.IsZero() {
		s.CleanupStart = now
	}
	return s
}

// CleanupExpired reports whether the cleanup window has run out at now.
func (s DispatchState) CleanupExpired(now time.Time) bool {
	return !s.CleanupStart.IsZero() && now.Sub(s.CleanupStart) >= DispatchWindow
}

// BackoffRemaining returns how long the next create must still wait at now:
// zero once NextAttemptAt has passed or when it is unset.
func (s DispatchState) BackoffRemaining(now time.Time) time.Duration {
	if s.NextAttemptAt.IsZero() || !now.Before(s.NextAttemptAt) {
		return 0
	}
	return s.NextAttemptAt.Sub(now)
}

// Metadata encodes the state as condition metadata: a Struct of the fields
// that are set, with times as RFC 3339 strings. The zero state encodes as
// nil, which clears the metadata.
func (s DispatchState) Metadata() (*protoTypes.Any, error) {
	if s.IsZero() {
		return nil, nil
	}
	fields := map[string]*protoTypes.Value{}
	setTimeField(fields, constants.DispatchWindowStart, s.WindowStart)
	setNumberField(fields, constants.DispatchAttempts, s.Attempts)
	if s.Ambiguous {
		fields[constants.DispatchAmbiguous] = &protoTypes.Value{Kind: &protoTypes.Value_BoolValue{BoolValue: true}}
	}
	setTimeField(fields, constants.NextAttemptAt, s.NextAttemptAt)
	setNumberField(fields, constants.Replacements, s.Replacements)
	if len(s.ExcludedClusters) > 0 {
		names := make([]*protoTypes.Value, 0, len(s.ExcludedClusters))
		for _, name := range s.ExcludedClusters {
			names = append(names, &protoTypes.Value{Kind: &protoTypes.Value_StringValue{StringValue: name}})
		}
		fields[constants.ExcludedClusters] = &protoTypes.Value{Kind: &protoTypes.Value_ListValue{ListValue: &protoTypes.ListValue{Values: names}}}
	}
	setTimeField(fields, constants.CleanupStart, s.CleanupStart)
	metadata, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: fields})
	if err != nil {
		return nil, fmt.Errorf("encode dispatch state: %w", err)
	}
	return metadata, nil
}

// DispatchStateFromCondition reads the state from a Launched condition. A
// nil condition, or one without metadata, yields the zero state; metadata
// written before excluded clusters were recorded yields none. Metadata that
// cannot be decoded yields the zero state and an error.
func DispatchStateFromCondition(cond *apipb.Condition) (DispatchState, error) {
	var s DispatchState
	if cond.GetMetadata() == nil {
		return s, nil
	}
	meta := &protoTypes.Struct{}
	if err := protoTypes.UnmarshalAny(cond.GetMetadata(), meta); err != nil {
		return DispatchState{}, fmt.Errorf("decode dispatch state: %w", err)
	}
	fields := meta.GetFields()
	s.Attempts = int(fields[constants.DispatchAttempts].GetNumberValue())
	s.Ambiguous = fields[constants.DispatchAmbiguous].GetBoolValue()
	s.Replacements = int(fields[constants.Replacements].GetNumberValue())
	for _, value := range fields[constants.ExcludedClusters].GetListValue().GetValues() {
		if name := value.GetStringValue(); name != "" {
			s.ExcludedClusters = append(s.ExcludedClusters, name)
		}
	}
	var err error
	if s.WindowStart, err = timeField(fields, constants.DispatchWindowStart); err != nil {
		return DispatchState{}, err
	}
	if s.NextAttemptAt, err = timeField(fields, constants.NextAttemptAt); err != nil {
		return DispatchState{}, err
	}
	if s.CleanupStart, err = timeField(fields, constants.CleanupStart); err != nil {
		return DispatchState{}, err
	}
	return s, nil
}

func setNumberField(fields map[string]*protoTypes.Value, key string, n int) {
	if n == 0 {
		return
	}
	fields[key] = &protoTypes.Value{Kind: &protoTypes.Value_NumberValue{NumberValue: float64(n)}}
}

func setTimeField(fields map[string]*protoTypes.Value, key string, t time.Time) {
	if t.IsZero() {
		return
	}
	fields[key] = &protoTypes.Value{Kind: &protoTypes.Value_StringValue{StringValue: t.UTC().Format(time.RFC3339Nano)}}
}

func timeField(fields map[string]*protoTypes.Value, key string) (time.Time, error) {
	raw := fields[key].GetStringValue()
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("decode dispatch state field %s: %w", key, err)
	}
	return t, nil
}
