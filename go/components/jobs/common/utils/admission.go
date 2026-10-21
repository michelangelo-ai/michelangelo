package utils

import (
	"fmt"
	"time"

	protoTypes "github.com/gogo/protobuf/types"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// IsKueueManaged reports whether Kueue admits the jobs dispatched to cluster:
// the k8s engine labels them with their LocalQueue.
func IsKueueManaged(cluster *v2pb.Cluster) bool {
	spec := cluster.GetSpec()
	return spec.GetSchedulerType() == v2pb.SCHEDULER_TYPE_KUEUE
}

// AdmissionObservation is what a status poll of a dispatched job says about
// its admission (RFC 0a).
type AdmissionObservation int

const (
	// ObservedOther is a state that says nothing about admission: the job
	// ended, or its state is not recognized. Queued is left as it is, so a
	// job that ends while suspended keeps Queued=True.
	ObservedOther AdmissionObservation = iota
	// ObservedSuspended is a job held suspended until it is admitted.
	ObservedSuspended
	// ObservedUnsuspended is a live job that is no longer suspended: it was
	// admitted.
	ObservedUnsuspended
)

// QueuedChange is a transition of a job's Queued condition (RFC 0a).
type QueuedChange struct {
	// Status and Reason are the condition's new status and reason:
	// True/AwaitingAdmission or False/ClusterAdmitted.
	Status apipb.ConditionStatus
	Reason string
	// WaitStart is the start of the wait for admission that begins, for
	// True, or that the admission ends, for False. Zero when unknown.
	WaitStart time.Time
}

// Admitted reports whether the change records an admission.
func (c QueuedChange) Admitted() bool {
	return c.Status == apipb.CONDITION_STATUS_FALSE
}

// Metadata returns the condition metadata of the change: the wait's start
// as admissionWaitStart for True, and nothing for False.
func (c QueuedChange) Metadata() (*protoTypes.Any, error) {
	if c.Status != apipb.CONDITION_STATUS_TRUE || c.WaitStart.IsZero() {
		return nil, nil
	}
	fields := map[string]*protoTypes.Value{}
	setTimeField(fields, constants.AdmissionWaitStart, c.WaitStart)
	metadata, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: fields})
	if err != nil {
		return nil, fmt.Errorf("encode admission wait start: %w", err)
	}
	return metadata, nil
}

// NextQueued returns the transition that an observation makes to a job's
// Queued condition, if it makes one. queued is the job's Queued condition, or
// nil when it has none: find it with FindCondition, since GetCondition adds
// one. created is the job's creation time.
//
// Queued is True exactly while the job is suspended, and it changes only on a
// transition, so that its timestamp is the time of the transition:
//   - suspended, and Queued not True: True/AwaitingAdmission. The wait starts
//     now after an eviction (Queued False), otherwise at the job's creation.
//   - unsuspended, and Queued True: False/ClusterAdmitted, which ends the wait
//     that started at the condition's admissionWaitStart.
//   - unsuspended on a Kueue-managed target, and no Queued condition:
//     False/ClusterAdmitted, once. The job was admitted before any poll saw
//     it suspended, so its wait started at creation. A job on any other
//     target never gets a Queued condition.
func NextQueued(queued *apipb.Condition, observed AdmissionObservation, kueueTarget bool, created, now time.Time) (QueuedChange, bool) {
	switch observed {
	case ObservedSuspended:
		switch queued.GetStatus() {
		case apipb.CONDITION_STATUS_TRUE:
			return QueuedChange{}, false
		case apipb.CONDITION_STATUS_FALSE:
			return QueuedChange{Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AwaitingAdmission, WaitStart: now}, true
		default:
			return QueuedChange{Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AwaitingAdmission, WaitStart: created}, true
		}
	case ObservedUnsuspended:
		switch {
		case queued.GetStatus() == apipb.CONDITION_STATUS_TRUE:
			return QueuedChange{Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.ClusterAdmitted, WaitStart: QueuedWaitStart(queued, created)}, true
		case queued == nil && kueueTarget:
			return QueuedChange{Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.ClusterAdmitted, WaitStart: created}, true
		}
	}
	return QueuedChange{}, false
}

// QueuedWaitStart returns the start of the wait for admission that a True
// Queued condition records in its admissionWaitStart. A condition without one,
// written before it was recorded, or whose metadata cannot be decoded, counts
// from created.
func QueuedWaitStart(queued *apipb.Condition, created time.Time) time.Time {
	if queued.GetMetadata() == nil {
		return created
	}
	meta := &protoTypes.Struct{}
	if err := protoTypes.UnmarshalAny(queued.GetMetadata(), meta); err != nil {
		return created
	}
	start, err := timeField(meta.GetFields(), constants.AdmissionWaitStart)
	if err != nil || start.IsZero() {
		return created
	}
	return start
}

// AdmissionWait is a job's wait for admission (RFC 0a).
type AdmissionWait struct {
	// Waiting reports whether the job waits for admission.
	Waiting bool
	// Start is when the wait started. Zero when the job does not wait, or
	// when the start is unknown.
	Start time.Time
}

// AdmissionWaitOf returns a job's wait for admission, given its conditions
// and creation time. A job waits while its Queued condition is True, that is
// while Kueue holds it suspended, under either backend; under the Kueue
// backend it also waits until it launches. The wait started at the latest
// eviction, which a True Queued condition records, and otherwise at the job's
// creation: a re-placement is not an eviction, so it does not restart the
// wait.
func AdmissionWaitOf(kueueBackend bool, conditions []*apipb.Condition, created time.Time) AdmissionWait {
	if queued := FindCondition(conditions, constants.QueuedCondition); queued.GetStatus() == apipb.CONDITION_STATUS_TRUE {
		return AdmissionWait{Waiting: true, Start: QueuedWaitStart(queued, created)}
	}
	if kueueBackend && FindCondition(conditions, constants.LaunchedCondition).GetStatus() != apipb.CONDITION_STATUS_TRUE {
		return AdmissionWait{Waiting: true, Start: created}
	}
	return AdmissionWait{}
}

// Expired reports whether the wait has reached bound at now. A wait whose
// start is unknown, which only an object that the API server never stamped
// can have, never expires.
func (w AdmissionWait) Expired(bound time.Duration, now time.Time) bool {
	return w.Waiting && !w.Start.IsZero() && now.Sub(w.Start) >= bound
}
