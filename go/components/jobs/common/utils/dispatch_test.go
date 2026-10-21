package utils

import (
	"testing"
	"time"

	protoTypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestDispatchBackoff(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: 5 * time.Second},
		{attempts: 1, want: 5 * time.Second},
		{attempts: 2, want: 10 * time.Second},
		{attempts: 3, want: 20 * time.Second},
		{attempts: 4, want: 40 * time.Second},
		{attempts: 5, want: 60 * time.Second},
		{attempts: 6, want: 60 * time.Second},
		{attempts: 100, want: 60 * time.Second},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, DispatchBackoff(tt.attempts), "attempts %d", tt.attempts)
	}
}

func TestDispatchStateWithAttempt(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	s := DispatchState{}.WithAttempt(start, false)
	assert.Equal(t, DispatchState{WindowStart: start, Attempts: 1, NextAttemptAt: start.Add(5 * time.Second)}, s)

	// A later failure keeps the window start, doubles the backoff and latches ambiguity.
	second := start.Add(7 * time.Second)
	s = s.WithAttempt(second, true)
	assert.Equal(t, DispatchState{
		WindowStart: start, Attempts: 2, Ambiguous: true, NextAttemptAt: second.Add(10 * time.Second),
	}, s)

	// Ambiguity never unlatches.
	third := start.Add(20 * time.Second)
	s = s.WithAttempt(third, false)
	assert.True(t, s.Ambiguous)
	assert.Equal(t, 3, s.Attempts)
	assert.Equal(t, third.Add(20*time.Second), s.NextAttemptAt)
}

func TestDispatchStateWindows(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name           string
		state          DispatchState
		now            time.Time
		windowExpired  bool
		cleanupExpired bool
		backoff        time.Duration
	}{
		{
			name: "zero state",
			now:  start,
		},
		{
			name:    "inside the window, inside the backoff",
			state:   DispatchState{WindowStart: start, Attempts: 1, NextAttemptAt: start.Add(5 * time.Second)},
			now:     start.Add(2 * time.Second),
			backoff: 3 * time.Second,
		},
		{
			name:  "backoff over exactly at nextAttemptAt",
			state: DispatchState{WindowStart: start, Attempts: 1, NextAttemptAt: start.Add(5 * time.Second)},
			now:   start.Add(5 * time.Second),
		},
		{
			name:  "one tick before the window ends",
			state: DispatchState{WindowStart: start, Attempts: 9},
			now:   start.Add(DispatchWindow - time.Nanosecond),
		},
		{
			name:          "window ends at 10 minutes",
			state:         DispatchState{WindowStart: start, Attempts: 9},
			now:           start.Add(DispatchWindow),
			windowExpired: true,
		},
		{
			name:  "cleanup one tick before its window ends",
			state: DispatchState{CleanupStart: start},
			now:   start.Add(DispatchWindow - time.Nanosecond),
		},
		{
			name:           "cleanup window ends at 10 minutes",
			state:          DispatchState{WindowStart: start.Add(-time.Hour), CleanupStart: start},
			now:            start.Add(DispatchWindow),
			windowExpired:  true,
			cleanupExpired: true,
		},
		{
			name:    "backoff after a re-placement",
			state:   DispatchState{Replacements: 1, NextAttemptAt: start.Add(10 * time.Second)},
			now:     start.Add(4 * time.Second),
			backoff: 6 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.windowExpired, tt.state.WindowExpired(tt.now))
			assert.Equal(t, tt.cleanupExpired, tt.state.CleanupExpired(tt.now))
			assert.Equal(t, tt.backoff, tt.state.BackoffRemaining(tt.now))
		})
	}
}

// TestDispatchStateWithReplacement: re-placement drops the window on the old
// target and keeps the backoff, which grows with each re-placement.
func TestDispatchStateWithReplacement(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	failing := DispatchState{WindowStart: start.Add(-time.Minute), Attempts: 3, NextAttemptAt: start.Add(20 * time.Second)}

	s := failing.WithReplacement(start, "cluster-a")
	assert.Equal(t, DispatchState{
		Replacements: 1, NextAttemptAt: start.Add(10 * time.Second), ExcludedClusters: []string{"cluster-a"},
	}, s)

	// The wait after each re-placement doubles up to the cap: 5s·2^r.
	var waits []time.Duration
	now := start
	for i := 0; i < 5; i++ {
		s = s.WithAttempt(now, false).WithReplacement(now, "cluster-a")
		waits = append(waits, s.NextAttemptAt.Sub(now))
		now = s.NextAttemptAt
	}
	assert.Equal(t, []time.Duration{20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second}, waits)
	assert.Equal(t, 6, s.Replacements)
	assert.Zero(t, s.Attempts)
	assert.True(t, s.WindowStart.IsZero())

	// Only the window fields are dropped.
	cleaning := DispatchState{
		WindowStart: start, Attempts: 1, Ambiguous: true, Replacements: 2,
		ExcludedClusters: []string{"cluster-a"}, CleanupStart: start,
	}
	s = cleaning.WithReplacement(start, "cluster-b")
	assert.Equal(t, DispatchState{
		Replacements: 3, NextAttemptAt: start.Add(40 * time.Second),
		ExcludedClusters: []string{"cluster-a", "cluster-b"}, CleanupStart: start,
	}, s)
}

// TestDispatchStateWithReplacementExcludesTheClustersLeft: each re-placement
// appends the cluster it leaves, in order and once.
func TestDispatchStateWithReplacementExcludesTheClustersLeft(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	var s DispatchState
	for _, left := range []string{"cluster-a", "cluster-b", "cluster-a", "cluster-c", "cluster-b"} {
		s = s.WithReplacement(start, left)
	}
	assert.Equal(t, []string{"cluster-a", "cluster-b", "cluster-c"}, s.ExcludedClusters)
	assert.Equal(t, 5, s.Replacements)

	// The state a re-placement starts from is unchanged, even when its list
	// has spare capacity.
	base := DispatchState{ExcludedClusters: make([]string, 1, 4)}
	base.ExcludedClusters[0] = "cluster-a"
	first := base.WithReplacement(start, "cluster-b")
	second := base.WithReplacement(start, "cluster-c")
	assert.Equal(t, []string{"cluster-a"}, base.ExcludedClusters)
	assert.Equal(t, []string{"cluster-a", "cluster-b"}, first.ExcludedClusters)
	assert.Equal(t, []string{"cluster-a", "cluster-c"}, second.ExcludedClusters)

	// A re-placement off no named cluster excludes nothing.
	assert.Empty(t, DispatchState{}.WithReplacement(start, "").ExcludedClusters)
}

func TestDispatchStateWithCleanupStart(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	dispatched := DispatchState{WindowStart: start.Add(-time.Minute), Attempts: 3, Ambiguous: true}

	s := dispatched.WithCleanupStart(start)
	assert.Equal(t, start, s.CleanupStart)
	// The dispatch record is kept.
	assert.Equal(t, 3, s.Attempts)
	assert.True(t, s.Ambiguous)

	// Written once: a later cleanup reconcile keeps the start.
	s = s.WithCleanupStart(start.Add(30 * time.Second))
	assert.Equal(t, start, s.CleanupStart)
}

func TestDispatchStateMetadataRoundTrip(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 123456789, time.UTC)
	tests := []struct {
		name  string
		state DispatchState
	}{
		{
			name:  "attempts only",
			state: DispatchState{WindowStart: start, Attempts: 1, NextAttemptAt: start.Add(5 * time.Second)},
		},
		{
			name:  "ambiguous",
			state: DispatchState{WindowStart: start, Attempts: 4, Ambiguous: true, NextAttemptAt: start.Add(time.Minute)},
		},
		{
			name:  "re-placed",
			state: DispatchState{Replacements: 2, NextAttemptAt: start.Add(20 * time.Second)},
		},
		{
			name:  "re-placed off earlier targets",
			state: DispatchState{Replacements: 2, NextAttemptAt: start.Add(20 * time.Second), ExcludedClusters: []string{"cluster-b", "cluster-a"}},
		},
		{
			name: "cleanup",
			state: DispatchState{
				WindowStart: start, Attempts: 2, Ambiguous: true,
				NextAttemptAt: start.Add(2 * time.Minute), Replacements: 1, ExcludedClusters: []string{"cluster-a"},
				CleanupStart: start.Add(time.Minute),
			},
		},
		{
			name:  "excluded clusters only",
			state: DispatchState{ExcludedClusters: []string{"cluster-a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := tt.state.Metadata()
			require.NoError(t, err)
			require.NotNil(t, metadata)
			got, err := DispatchStateFromCondition(&apipb.Condition{Metadata: metadata})
			require.NoError(t, err)
			assert.Equal(t, tt.state, got)
		})
	}
}

func TestDispatchStateMetadataEncoding(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	metadata, err := DispatchState{WindowStart: start, Attempts: 2, NextAttemptAt: start.Add(10 * time.Second)}.Metadata()
	require.NoError(t, err)

	meta := &protoTypes.Struct{}
	require.NoError(t, protoTypes.UnmarshalAny(metadata, meta))
	fields := meta.GetFields()
	assert.Equal(t, "2026-10-20T12:00:00Z", fields[constants.DispatchWindowStart].GetStringValue())
	assert.Equal(t, 2.0, fields[constants.DispatchAttempts].GetNumberValue())
	assert.Equal(t, "2026-10-20T12:00:10Z", fields[constants.NextAttemptAt].GetStringValue())
	assert.Len(t, fields, 3)

	// After a re-placement only the re-placements, the backoff and the
	// excluded clusters are left.
	metadata, err = DispatchState{WindowStart: start, Attempts: 2, Ambiguous: true}.WithReplacement(start, "cluster-a").Metadata()
	require.NoError(t, err)
	meta = &protoTypes.Struct{}
	require.NoError(t, protoTypes.UnmarshalAny(metadata, meta))
	assert.Equal(t, map[string]*protoTypes.Value{
		constants.Replacements:  {Kind: &protoTypes.Value_NumberValue{NumberValue: 1}},
		constants.NextAttemptAt: {Kind: &protoTypes.Value_StringValue{StringValue: "2026-10-20T12:00:10Z"}},
		constants.ExcludedClusters: {Kind: &protoTypes.Value_ListValue{ListValue: &protoTypes.ListValue{Values: []*protoTypes.Value{
			{Kind: &protoTypes.Value_StringValue{StringValue: "cluster-a"}},
		}}}},
	}, meta.GetFields())

	// The zero state clears the metadata.
	metadata, err = DispatchState{}.Metadata()
	require.NoError(t, err)
	assert.Nil(t, metadata)
}

func TestDispatchStateFromCondition(t *testing.T) {
	state, err := DispatchStateFromCondition(nil)
	require.NoError(t, err)
	assert.True(t, state.IsZero())

	state, err = DispatchStateFromCondition(&apipb.Condition{Type: constants.LaunchedCondition})
	require.NoError(t, err)
	assert.True(t, state.IsZero())

	// Metadata of another shape is not a dispatch state.
	notAStruct, err := protoTypes.MarshalAny(&protoTypes.StringValue{Value: "x"})
	require.NoError(t, err)
	state, err = DispatchStateFromCondition(&apipb.Condition{Metadata: notAStruct})
	require.Error(t, err)
	assert.True(t, state.IsZero())

	// Metadata written before excluded clusters were recorded has none.
	older, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: map[string]*protoTypes.Value{
		constants.Replacements:  {Kind: &protoTypes.Value_NumberValue{NumberValue: 2}},
		constants.NextAttemptAt: {Kind: &protoTypes.Value_StringValue{StringValue: "2026-10-20T12:00:20Z"}},
	}})
	require.NoError(t, err)
	state, err = DispatchStateFromCondition(&apipb.Condition{Metadata: older})
	require.NoError(t, err)
	assert.Equal(t, DispatchState{Replacements: 2, NextAttemptAt: time.Date(2026, 10, 20, 12, 0, 20, 0, time.UTC)}, state)
	assert.Empty(t, state.ExcludedClusters)

	badTime, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: map[string]*protoTypes.Value{
		constants.NextAttemptAt: {Kind: &protoTypes.Value_StringValue{StringValue: "soon"}},
	}})
	require.NoError(t, err)
	state, err = DispatchStateFromCondition(&apipb.Condition{Metadata: badTime})
	require.Error(t, err)
	assert.True(t, state.IsZero())
}
