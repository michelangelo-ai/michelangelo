package common

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// RolloutProgress is the bookkeeping an actor keeps on its condition's Metadata across
// reconciles. Conditions are persisted in the Deployment status, so this is how an actor
// remembers when it started waiting, which replica it pinned its work to, and that its
// goal was already observed, without re-probing the cluster on every reconcile.
type RolloutProgress struct {
	// StartedAt is when the actor first kicked off its work, in Unix seconds. Zero means
	// Run has not been called yet.
	StartedAt int64 `json:"startedAt,omitempty"`
	// Done records that the actor observed its goal as reached.
	Done bool `json:"done,omitempty"`
	// CompletedAt is when Done was set, in Unix seconds.
	CompletedAt int64 `json:"completedAt,omitempty"`
	// Replica is the replica the actor pinned its work to, e.g. the canary pod.
	Replica string `json:"replica,omitempty"`
}

// Started reports whether the actor has recorded a start time.
func (p RolloutProgress) Started() bool {
	return p.StartedAt > 0
}

// Elapsed returns how long the actor has been working, or zero if it has not started.
func (p RolloutProgress) Elapsed(now time.Time) time.Duration {
	if !p.Started() {
		return 0
	}
	return now.Sub(time.Unix(p.StartedAt, 0))
}

// TimedOut reports whether the actor has been working for longer than timeout. A zero
// timeout never expires.
func (p RolloutProgress) TimedOut(now time.Time, timeout time.Duration) bool {
	return timeout > 0 && p.Started() && p.Elapsed(now) > timeout
}

// ReadRolloutProgress decodes the progress stored on the condition. A condition without
// metadata yields the zero value. A legacy BoolValue written by an older controller is read
// as Done, so an in-flight rollout keeps its place across an upgrade.
func ReadRolloutProgress(condition *apipb.Condition) (RolloutProgress, error) {
	var progress RolloutProgress
	if condition == nil || condition.Metadata == nil {
		return progress, nil
	}

	if types.Is(condition.Metadata, &types.BoolValue{}) {
		val := &types.BoolValue{}
		if err := types.UnmarshalAny(condition.Metadata, val); err != nil {
			return progress, fmt.Errorf("decode legacy loaded flag: %w", err)
		}
		progress.Done = val.Value
		return progress, nil
	}

	val := &types.StringValue{}
	if err := types.UnmarshalAny(condition.Metadata, val); err != nil {
		return progress, fmt.Errorf("decode rollout progress: %w", err)
	}
	if err := json.Unmarshal([]byte(val.Value), &progress); err != nil {
		return progress, fmt.Errorf("parse rollout progress: %w", err)
	}
	return progress, nil
}

// WriteRolloutProgress stores the progress on the condition's Metadata.
func WriteRolloutProgress(condition *apipb.Condition, progress RolloutProgress) error {
	raw, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("encode rollout progress: %w", err)
	}
	metadata, err := types.MarshalAny(&types.StringValue{Value: string(raw)})
	if err != nil {
		return fmt.Errorf("wrap rollout progress: %w", err)
	}
	condition.Metadata = metadata
	return nil
}
