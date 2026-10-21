package utils

import (
	"testing"
	"time"

	protoTypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var (
	admissionCreated = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	admissionNow     = time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	admissionEvicted = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
)

// queuedCondition is a Queued condition whose metadata records waitStart,
// unless it is zero.
func queuedCondition(t *testing.T, status apipb.ConditionStatus, reason string, waitStart time.Time) *apipb.Condition {
	t.Helper()
	metadata, err := QueuedChange{Status: status, WaitStart: waitStart}.Metadata()
	require.NoError(t, err)
	return &apipb.Condition{Type: constants.QueuedCondition, Status: status, Reason: reason, Metadata: metadata}
}

func TestIsKueueManaged(t *testing.T) {
	assert.True(t, IsKueueManaged(&v2pb.Cluster{Spec: v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE}}))
	assert.False(t, IsKueueManaged(&v2pb.Cluster{}))
	assert.False(t, IsKueueManaged(nil))
}

func TestNextQueued(t *testing.T) {
	trueSinceEviction := queuedCondition(t, apipb.CONDITION_STATUS_TRUE, constants.AwaitingAdmission, admissionEvicted)
	trueWithoutStart := queuedCondition(t, apipb.CONDITION_STATUS_TRUE, constants.AwaitingAdmission, time.Time{})
	admitted := queuedCondition(t, apipb.CONDITION_STATUS_FALSE, constants.ClusterAdmitted, time.Time{})
	unknown := &apipb.Condition{Type: constants.QueuedCondition, Status: apipb.CONDITION_STATUS_UNKNOWN}

	awaiting := func(start time.Time) *QueuedChange {
		return &QueuedChange{Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AwaitingAdmission, WaitStart: start}
	}
	admittedAfter := func(start time.Time) *QueuedChange {
		return &QueuedChange{Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.ClusterAdmitted, WaitStart: start}
	}

	tests := []struct {
		name        string
		queued      *apipb.Condition
		observed    AdmissionObservation
		kueueTarget bool
		want        *QueuedChange // nil: no change
	}{
		{name: "first suspension waits from creation", observed: ObservedSuspended, want: awaiting(admissionCreated)},
		{name: "first suspension on a Kueue target", observed: ObservedSuspended, kueueTarget: true, want: awaiting(admissionCreated)},
		{name: "an eviction waits from now", queued: admitted, observed: ObservedSuspended, kueueTarget: true, want: awaiting(admissionNow)},
		{name: "Unknown Queued while suspended waits from creation", queued: unknown, observed: ObservedSuspended, want: awaiting(admissionCreated)},
		{name: "still suspended: no restamp", queued: trueSinceEviction, observed: ObservedSuspended, kueueTarget: true},
		{name: "admission ends the wait since the eviction", queued: trueSinceEviction, observed: ObservedUnsuspended, kueueTarget: true, want: admittedAfter(admissionEvicted)},
		{name: "admission without a recorded start counts from creation", queued: trueWithoutStart, observed: ObservedUnsuspended, want: admittedAfter(admissionCreated)},
		{name: "admitted before any poll saw it suspended: once-only marker", observed: ObservedUnsuspended, kueueTarget: true, want: admittedAfter(admissionCreated)},
		{name: "marker already written", queued: admitted, observed: ObservedUnsuspended, kueueTarget: true},
		{name: "a non-Kueue target never gets Queued", observed: ObservedUnsuspended},
		{name: "Unknown Queued is not absent", queued: unknown, observed: ObservedUnsuspended, kueueTarget: true},
		{name: "ended while suspended keeps Queued True", queued: trueSinceEviction, observed: ObservedOther, kueueTarget: true},
		{name: "ended before any poll on a Kueue target", observed: ObservedOther, kueueTarget: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NextQueued(tt.queued, tt.observed, tt.kueueTarget, admissionCreated, admissionNow)
			if tt.want == nil {
				assert.False(t, ok)
				assert.Equal(t, QueuedChange{}, got)
				return
			}
			require.True(t, ok)
			assert.Equal(t, *tt.want, got)
			assert.Equal(t, tt.want.Status == apipb.CONDITION_STATUS_FALSE, got.Admitted())
		})
	}
}

func TestQueuedChangeMetadata(t *testing.T) {
	metadata, err := QueuedChange{Status: apipb.CONDITION_STATUS_TRUE, WaitStart: admissionEvicted}.Metadata()
	require.NoError(t, err)
	meta := &protoTypes.Struct{}
	require.NoError(t, protoTypes.UnmarshalAny(metadata, meta))
	assert.Equal(t, "2026-10-20T08:00:00Z", meta.GetFields()[constants.AdmissionWaitStart].GetStringValue())

	// A False condition, or an unknown start, records nothing.
	metadata, err = QueuedChange{Status: apipb.CONDITION_STATUS_FALSE, WaitStart: admissionEvicted}.Metadata()
	require.NoError(t, err)
	assert.Nil(t, metadata)
	metadata, err = QueuedChange{Status: apipb.CONDITION_STATUS_TRUE}.Metadata()
	require.NoError(t, err)
	assert.Nil(t, metadata)
}

func TestQueuedWaitStart(t *testing.T) {
	assert.Equal(t, admissionEvicted,
		QueuedWaitStart(queuedCondition(t, apipb.CONDITION_STATUS_TRUE, constants.AwaitingAdmission, admissionEvicted), admissionCreated))
	assert.Equal(t, admissionCreated, QueuedWaitStart(&apipb.Condition{Type: constants.QueuedCondition}, admissionCreated))
	assert.Equal(t, admissionCreated, QueuedWaitStart(nil, admissionCreated))

	// Metadata of another kind, or a malformed start, counts from creation.
	other, err := protoTypes.MarshalAny(&protoTypes.Empty{})
	require.NoError(t, err)
	assert.Equal(t, admissionCreated, QueuedWaitStart(&apipb.Condition{Metadata: other}, admissionCreated))
	malformed, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: map[string]*protoTypes.Value{
		constants.AdmissionWaitStart: {Kind: &protoTypes.Value_StringValue{StringValue: "yesterday"}},
	}})
	require.NoError(t, err)
	assert.Equal(t, admissionCreated, QueuedWaitStart(&apipb.Condition{Metadata: malformed}, admissionCreated))
}

func TestAdmissionWaitOf(t *testing.T) {
	launched := &apipb.Condition{Type: constants.LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE}
	notLaunched := &apipb.Condition{Type: constants.LaunchedCondition, Status: apipb.CONDITION_STATUS_FALSE}
	suspended := queuedCondition(t, apipb.CONDITION_STATUS_TRUE, constants.AwaitingAdmission, admissionEvicted)
	admitted := queuedCondition(t, apipb.CONDITION_STATUS_FALSE, constants.ClusterAdmitted, time.Time{})

	tests := []struct {
		name       string
		kueue      bool
		conditions []*apipb.Condition
		want       AdmissionWait
	}{
		{name: "kueue: before dispatch, from creation", kueue: true, want: AdmissionWait{Waiting: true, Start: admissionCreated}},
		{name: "kueue: dispatch failing, from creation", kueue: true, conditions: []*apipb.Condition{notLaunched}, want: AdmissionWait{Waiting: true, Start: admissionCreated}},
		{name: "kueue: launched on a non-Kueue target", kueue: true, conditions: []*apipb.Condition{launched}},
		{name: "kueue: suspended, from the eviction", kueue: true, conditions: []*apipb.Condition{launched, suspended}, want: AdmissionWait{Waiting: true, Start: admissionEvicted}},
		{name: "kueue: admitted", kueue: true, conditions: []*apipb.Condition{launched, admitted}},
		{name: "default: suspended on a Kueue-marked target", conditions: []*apipb.Condition{launched, suspended}, want: AdmissionWait{Waiting: true, Start: admissionEvicted}},
		{name: "default: before dispatch, never", conditions: []*apipb.Condition{notLaunched}},
		{name: "default: never suspended, never"},
		{name: "default: admitted", conditions: []*apipb.Condition{launched, admitted}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AdmissionWaitOf(tt.kueue, tt.conditions, admissionCreated))
		})
	}
}

func TestAdmissionWaitExpired(t *testing.T) {
	bound := 24 * time.Hour
	start := admissionNow.Add(-bound)
	tests := []struct {
		name string
		wait AdmissionWait
		now  time.Time
		want bool
	}{
		{name: "inside the bound", wait: AdmissionWait{Waiting: true, Start: start}, now: admissionNow.Add(-time.Second)},
		{name: "at the bound", wait: AdmissionWait{Waiting: true, Start: start}, now: admissionNow, want: true},
		{name: "past the bound", wait: AdmissionWait{Waiting: true, Start: start}, now: admissionNow.Add(time.Hour), want: true},
		{name: "not waiting", wait: AdmissionWait{Start: start}, now: admissionNow.Add(time.Hour)},
		{name: "unknown start never expires", wait: AdmissionWait{Waiting: true}, now: admissionNow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.wait.Expired(bound, tt.now))
		})
	}
}
