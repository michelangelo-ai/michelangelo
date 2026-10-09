package common

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestReadRolloutProgress_NoMetadata(t *testing.T) {
	got, err := ReadRolloutProgress(&apipb.Condition{})

	require.NoError(t, err)
	assert.Equal(t, RolloutProgress{}, got)
	assert.False(t, got.Started())
	assert.False(t, got.Done)

	got, err = ReadRolloutProgress(nil)
	require.NoError(t, err)
	assert.Equal(t, RolloutProgress{}, got)
}

// A controller upgraded mid-rollout still has conditions carrying the old BoolValue flag;
// they must keep reading as "done" so the rollout does not restart from scratch.
func TestReadRolloutProgress_LegacyLoadedFlag(t *testing.T) {
	legacy, err := types.MarshalAny(&types.BoolValue{Value: true})
	require.NoError(t, err)

	got, err := ReadRolloutProgress(&apipb.Condition{Metadata: legacy})

	require.NoError(t, err)
	assert.True(t, got.Done)
	assert.False(t, got.Started())
}

func TestReadRolloutProgress_Malformed(t *testing.T) {
	notJSON, err := types.MarshalAny(&types.StringValue{Value: "{not json"})
	require.NoError(t, err)
	_, err = ReadRolloutProgress(&apipb.Condition{Metadata: notJSON})
	assert.Error(t, err, "unparseable progress must surface, not silently reset the rollout")

	wrongType, err := types.MarshalAny(&types.Int64Value{Value: 7})
	require.NoError(t, err)
	_, err = ReadRolloutProgress(&apipb.Condition{Metadata: wrongType})
	assert.Error(t, err)
}

func TestRolloutProgress_RoundTrip(t *testing.T) {
	condition := &apipb.Condition{Status: apipb.CONDITION_STATUS_UNKNOWN}
	want := RolloutProgress{StartedAt: 1_700_000_000, Done: true, CompletedAt: 1_700_000_100, Replica: "pod-a"}

	require.NoError(t, WriteRolloutProgress(condition, want))
	require.NotNil(t, condition.Metadata)
	got, err := ReadRolloutProgress(condition)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status, "writing progress must not touch the status")
}

func TestRolloutProgress_Timing(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	progress := RolloutProgress{StartedAt: start.Unix()}

	assert.True(t, progress.Started())
	assert.Equal(t, 90*time.Second, progress.Elapsed(start.Add(90*time.Second)))
	assert.False(t, progress.TimedOut(start.Add(time.Minute), 2*time.Minute))
	assert.False(t, progress.TimedOut(start.Add(2*time.Minute), 2*time.Minute), "the budget is inclusive")
	assert.True(t, progress.TimedOut(start.Add(2*time.Minute+time.Second), 2*time.Minute))
	assert.False(t, progress.TimedOut(start.Add(time.Hour), 0), "a zero budget never expires")

	var unstarted RolloutProgress
	assert.Equal(t, time.Duration(0), unstarted.Elapsed(start))
	assert.False(t, unstarted.TimedOut(start, time.Minute), "unstarted work cannot time out")
}
