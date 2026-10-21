package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	admissionTimeoutsMetric = "michelangelo_jobs_admission_timeouts_total"
	admissionWaitMetric     = "michelangelo_jobs_admission_wait_seconds"

	// testAdmissionTimeout is the admission bound the tests configure.
	testAdmissionTimeout = 2 * time.Hour
	// unwrittenTimestamp marks a condition the reconciler has not written:
	// a write stamps the wall-clock time.
	unwrittenTimestamp = 1
)

// rayClusterCreated is when the tests' RayClusters were created, an hour
// before the harness clock starts.
var rayClusterCreated = dispatchEpoch.Add(-time.Hour)

// newAdmissionHarness is a dispatch harness whose reconciler bounds the wait
// for admission at testAdmissionTimeout.
func newAdmissionHarness(t *testing.T, backend string, rc *v2pb.RayCluster) *dispatchHarness {
	t.Helper()
	h := newDispatchHarness(t, backend, rc)
	h.r.schedulerConfig.Kueue.AdmissionTimeout = testAdmissionTimeout
	return h
}

// createdAt stamps rc's creation time, as the API server does.
func createdAt(rc *v2pb.RayCluster) *v2pb.RayCluster {
	rc.CreationTimestamp = metav1.NewTime(rayClusterCreated)
	return rc
}

// newRayCluster is a RayCluster the reconciler has not seen yet, with the
// given conditions.
func newRayCluster(conditions ...*apipb.Condition) *v2pb.RayCluster {
	return createdAt(&v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1, UID: rayClusterUID},
		Spec:       createRayClusterSpec(),
		Status:     v2pb.RayClusterStatus{StatusConditions: conditions},
	})
}

// launchedRayCluster is a RayCluster launched on dispatchTarget, last seen in
// state, with the given further conditions. Its Succeeded condition is
// present, so that a poll that changes nothing writes nothing.
func launchedRayCluster(state v2pb.RayClusterState, conditions ...*apipb.Condition) *v2pb.RayCluster {
	rc := createdAt(scheduledRayCluster(&apipb.Condition{
		Type: LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: clusterCreatedReason,
	}))
	rc.Status.State = state
	rc.Status.StatusConditions = append(rc.Status.StatusConditions,
		&apipb.Condition{Type: SucceededCondition, Status: apipb.CONDITION_STATUS_UNKNOWN})
	rc.Status.StatusConditions = append(rc.Status.StatusConditions, conditions...)
	return rc
}

// queuedSince is the Queued=True condition of a wait that started at start.
func queuedSince(t *testing.T, start time.Time) *apipb.Condition {
	t.Helper()
	metadata, err := jobsutils.QueuedChange{Status: apipb.CONDITION_STATUS_TRUE, WaitStart: start}.Metadata()
	require.NoError(t, err)
	return &apipb.Condition{
		Type: QueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AwaitingAdmission,
		Metadata: metadata, LastUpdatedTimestamp: unwrittenTimestamp,
	}
}

// admitted is the Queued=False condition of an admission.
func admitted() *apipb.Condition {
	return &apipb.Condition{
		Type: QueuedCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.ClusterAdmitted,
		LastUpdatedTimestamp: unwrittenTimestamp,
	}
}

// onTarget registers dispatchTarget as Ready, Kueue-managed or not.
func (h *dispatchHarness) onTarget(kueueManaged bool) {
	target := registeredCluster(dispatchTarget)
	if kueueManaged {
		target.Spec.SchedulerType = v2pb.SCHEDULER_TYPE_KUEUE
	}
	h.readiness[dispatchTarget] = jobscluster.ReadinessReady
	h.cache.addCluster(dispatchTarget, target)
}

// statusPoll is what the status polls of the RayCluster on its target report.
type statusPoll struct {
	state v2pb.RayClusterState
}

// pollsReport makes the status polls report state, until the returned
// statusPoll is changed.
func (h *dispatchHarness) pollsReport(state v2pb.RayClusterState) *statusPoll {
	poll := &statusPoll{state: state}
	h.fed.EXPECT().GetJobClusterStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, runtime.Object, *v2pb.Cluster) (*matypes.JobClusterStatus, error) {
			status := &matypes.JobClusterStatus{Ray: &v2pb.RayClusterStatus{State: poll.state}}
			if poll.state == v2pb.RAY_CLUSTER_STATE_SUSPENDED {
				status.Reason = "ClusterSuspended"
			}
			return status, nil
		}).AnyTimes()
	return poll
}

// queued returns the stored Queued condition, or nil.
func (h *dispatchHarness) queued() *apipb.Condition {
	h.t.Helper()
	return jobsutils.FindCondition(h.get().Status.StatusConditions, QueuedCondition)
}

// markQueuedUnwritten sets the stored Queued condition's timestamp to
// unwrittenTimestamp, so that a later write of it shows.
func (h *dispatchHarness) markQueuedUnwritten() {
	h.t.Helper()
	h.update(func(rc *v2pb.RayCluster) {
		jobsutils.FindCondition(rc.Status.StatusConditions, QueuedCondition).LastUpdatedTimestamp = unwrittenTimestamp
	})
}

// reconcileUntilFinalized reconciles until the RayCluster is finalized. The
// termination flow takes a reconcile per status write: in production, each
// write's watch event triggers the next.
func (h *dispatchHarness) reconcileUntilFinalized() {
	h.t.Helper()
	for range 3 {
		h.reconcile()
		if utils.IsImmutable(h.get()) {
			return
		}
	}
	h.t.Fatal("the ray cluster was not finalized")
}

// countEnqueues makes the scheduler queue count the jobs enqueued on it.
func (h *dispatchHarness) countEnqueues() *int {
	var enqueues int
	h.r.schedulerQueue = &mockSchedulerQueue{enqueueFunc: func(context.Context, matypes.SchedulableJob) error {
		enqueues++
		return nil
	}}
	return &enqueues
}

// admissionMetrics is a reading of the admission metrics.
type admissionMetrics struct {
	waits    uint64  // admission waits observed
	waitSum  float64 // their total, in seconds
	timeouts float64 // admission timeouts
}

func readAdmissionMetrics(t *testing.T) admissionMetrics {
	t.Helper()
	m := admissionMetrics{
		timeouts: dispatchCounter(t, admissionTimeoutsMetric, map[string]string{"job_type": dispatchMetricsJobType}),
	}
	families, err := metrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != admissionWaitMetric {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metric.GetLabel()
			if len(labels) == 1 && labels[0].GetName() == "job_type" && labels[0].GetValue() == dispatchMetricsJobType {
				m.waits, m.waitSum = metric.GetHistogram().GetSampleCount(), metric.GetHistogram().GetSampleSum()
			}
		}
	}
	return m
}

// since returns what was recorded after before was read.
func (m admissionMetrics) since(before admissionMetrics) admissionMetrics {
	return admissionMetrics{waits: m.waits - before.waits, waitSum: m.waitSum - before.waitSum, timeouts: m.timeouts - before.timeouts}
}

// assertAdmissionTimeout checks that the RayCluster was failed for waiting
// waited for admission.
func assertAdmissionTimeout(t *testing.T, rc *v2pb.RayCluster, waited time.Duration) {
	t.Helper()
	succeeded := jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition)
	require.NotNil(t, succeeded)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
	assert.Equal(t, constants.AdmissionTimeout, succeeded.Reason)
	assert.Equal(t, "waited "+waited.String()+" for admission, reaching the admission timeout of 2h0m0s", succeeded.Message)
}

func assertNotFailed(t *testing.T, rc *v2pb.RayCluster) {
	t.Helper()
	assert.NotEqual(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
	assert.NotEqual(t, apipb.CONDITION_STATUS_FALSE, jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition).GetStatus())
}

func TestAdmissionTimeoutConfig(t *testing.T) {
	r := NewReconciler(logr.Discard(), nil, env.Context{}, &mockSchedulerQueue{}, nil, newMockClusterCache())
	assert.Equal(t, 24*time.Hour, r.admissionTimeout())

	r = NewReconciler(logr.Discard(), nil, env.Context{}, &mockSchedulerQueue{}, nil, newMockClusterCache(),
		WithSchedulerConfig(maconfig.SchedulerConfig{Kueue: maconfig.KueueConfig{AdmissionTimeout: 2 * time.Minute}}))
	assert.Equal(t, 2*time.Minute, r.admissionTimeout())
}

func TestAdmissionObservation(t *testing.T) {
	assert.Equal(t, jobsutils.ObservedSuspended, admissionObservation(v2pb.RAY_CLUSTER_STATE_SUSPENDED))
	for _, state := range []v2pb.RayClusterState{
		v2pb.RAY_CLUSTER_STATE_UNKNOWN, v2pb.RAY_CLUSTER_STATE_PROVISIONING,
		v2pb.RAY_CLUSTER_STATE_READY, v2pb.RAY_CLUSTER_STATE_UNHEALTHY,
	} {
		assert.Equal(t, jobsutils.ObservedUnsuspended, admissionObservation(state), state.String())
	}
	for _, state := range []v2pb.RayClusterState{
		v2pb.RAY_CLUSTER_STATE_INVALID, v2pb.RAY_CLUSTER_STATE_TERMINATING,
		v2pb.RAY_CLUSTER_STATE_TERMINATED, v2pb.RAY_CLUSTER_STATE_FAILED,
	} {
		assert.Equal(t, jobsutils.ObservedOther, admissionObservation(state), state.String())
	}
}

func TestQueuedIsWrittenOnlyOnATransition(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_PROVISIONING))
	h.onTarget(true)
	h.pollsReport(v2pb.RAY_CLUSTER_STATE_SUSPENDED)

	h.reconcile()
	queued := h.queued()
	require.NotNil(t, queued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, queued.Status)
	assert.Equal(t, constants.AwaitingAdmission, queued.Reason)
	assert.Equal(t, "ClusterSuspended", queued.Message)
	assert.WithinDuration(t, rayClusterCreated, jobsutils.QueuedWaitStart(queued, time.Time{}), 0, "the first wait starts at creation")
	assert.Equal(t, 1, h.statusWrites)

	// Polls that see the cluster still suspended write nothing, so that
	// Queued keeps the time of its transition.
	h.markQueuedUnwritten()
	for range 3 {
		h.advance(time.Minute)
		h.reconcile()
	}
	assert.Equal(t, 1, h.statusWrites)
	assert.Equal(t, int64(unwrittenTimestamp), h.queued().LastUpdatedTimestamp)
}

func TestQueuedFlipsOnceOnAdmission(t *testing.T) {
	for _, state := range []v2pb.RayClusterState{v2pb.RAY_CLUSTER_STATE_UNKNOWN, v2pb.RAY_CLUSTER_STATE_READY} {
		t.Run(state.String(), func(t *testing.T) {
			waitStart := dispatchEpoch.Add(-10 * time.Minute)
			h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_SUSPENDED, queuedSince(t, waitStart)))
			h.onTarget(true)
			h.pollsReport(state)
			before := readAdmissionMetrics(t)

			h.reconcile()
			queued := h.queued()
			require.NotNil(t, queued)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, queued.Status)
			assert.Equal(t, constants.ClusterAdmitted, queued.Reason)
			assert.Empty(t, queued.Message)
			assert.Nil(t, queued.Metadata)
			assert.NotEqual(t, int64(unwrittenTimestamp), queued.LastUpdatedTimestamp)
			assert.Equal(t, admissionMetrics{waits: 1, waitSum: 600}, readAdmissionMetrics(t).since(before),
				"the admission ends the wait that started at admissionWaitStart")

			h.markQueuedUnwritten()
			h.advance(time.Minute)
			h.reconcile()
			assert.Equal(t, int64(unwrittenTimestamp), h.queued().LastUpdatedTimestamp, "an admission is written once")
			assert.Equal(t, admissionMetrics{waits: 1, waitSum: 600}, readAdmissionMetrics(t).since(before), "an admission is observed once")
		})
	}
}

func TestQueuedBeforeAnySuspension(t *testing.T) {
	t.Run("target not Kueue-managed", func(t *testing.T) {
		h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_UNKNOWN))
		h.onTarget(false)
		h.pollsReport(v2pb.RAY_CLUSTER_STATE_UNKNOWN)
		before := readAdmissionMetrics(t)

		h.reconcile()
		h.reconcile()
		assert.Nil(t, h.queued())
		assert.Zero(t, h.statusWrites)
		assert.Equal(t, admissionMetrics{}, readAdmissionMetrics(t).since(before))
	})

	t.Run("Kueue-managed target", func(t *testing.T) {
		h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_UNKNOWN))
		h.onTarget(true)
		h.pollsReport(v2pb.RAY_CLUSTER_STATE_UNKNOWN)
		before := readAdmissionMetrics(t)

		// Kueue admitted the cluster before any poll saw it suspended.
		h.reconcile()
		queued := h.queued()
		require.NotNil(t, queued)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, queued.Status)
		assert.Equal(t, constants.ClusterAdmitted, queued.Reason)
		assert.Equal(t, 1, h.statusWrites)
		assert.Equal(t, admissionMetrics{waits: 1, waitSum: 3600}, readAdmissionMetrics(t).since(before), "the wait started at creation")

		h.markQueuedUnwritten()
		h.advance(time.Minute)
		h.reconcile()
		assert.Equal(t, 1, h.statusWrites, "the marker is written once")
		assert.Equal(t, int64(unwrittenTimestamp), h.queued().LastUpdatedTimestamp)
		assert.Equal(t, admissionMetrics{waits: 1, waitSum: 3600}, readAdmissionMetrics(t).since(before))
	})
}

func TestEvictionRestartsTheAdmissionWait(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_READY, admitted()))
	h.onTarget(true)
	poll := h.pollsReport(v2pb.RAY_CLUSTER_STATE_READY)
	before := readAdmissionMetrics(t)

	// Admitted, the cluster runs past the bound.
	h.advance(testAdmissionTimeout)
	h.reconcile()
	assertNotFailed(t, h.get())

	// Evicted, it waits again, from now.
	poll.state = v2pb.RAY_CLUSTER_STATE_SUSPENDED
	h.reconcile()
	queued := h.queued()
	require.NotNil(t, queued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, queued.Status)
	assert.Equal(t, constants.AwaitingAdmission, queued.Reason)
	assert.WithinDuration(t, h.now, jobsutils.QueuedWaitStart(queued, time.Time{}), 0, "an eviction starts a new wait")

	h.advance(testAdmissionTimeout - time.Second)
	h.reconcile()
	assertNotFailed(t, h.get())

	h.advance(time.Second)
	h.reconcile()
	assertAdmissionTimeout(t, h.get(), testAdmissionTimeout)
	assert.Equal(t, admissionMetrics{timeouts: 1}, readAdmissionMetrics(t).since(before))
}

func TestAdmissionTimeoutRemovesASuspendedCluster(t *testing.T) {
	for _, tc := range []struct{ name, backend string }{
		{name: "default backend", backend: ""},
		{name: "kueue backend", backend: backendKueue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdmissionHarness(t, tc.backend, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_SUSPENDED, queuedSince(t, rayClusterCreated)))
			h.onTarget(true)
			h.pollsReport(v2pb.RAY_CLUSTER_STATE_SUSPENDED)
			before := readAdmissionMetrics(t)

			h.advance(time.Hour - time.Second)
			h.reconcile()
			assertNotFailed(t, h.get())

			h.advance(time.Second)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			rc := h.get()
			assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
			assertAdmissionTimeout(t, rc, testAdmissionTimeout)
			assert.Equal(t, admissionMetrics{timeouts: 1}, readAdmissionMetrics(t).since(before),
				"a timeout is counted, and not observed as an admission")

			// The termination flow removes the cluster from its target.
			h.fed.EXPECT().DeleteJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			h.reconcileUntilFinalized()
			rc = h.get()
			assertFinalized(t, rc)
			assertAdmissionTimeout(t, rc, testAdmissionTimeout)
			assert.Equal(t, admissionMetrics{timeouts: 1}, readAdmissionMetrics(t).since(before))
		})
	}
}

func TestAdmissionTimeoutFailsAClusterThatNeverLaunched(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, newRayCluster())
	enqueues := h.countEnqueues()
	before := readAdmissionMetrics(t)

	// Not placed yet, the cluster waits.
	h.advance(time.Hour - time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	assertNotFailed(t, h.get())
	assert.Equal(t, 1, *enqueues)

	h.advance(time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	rc := h.get()
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
	assertAdmissionTimeout(t, rc, testAdmissionTimeout)
	assert.Equal(t, 1, *enqueues, "a cluster whose wait is over is not enqueued")
	assert.Equal(t, admissionMetrics{timeouts: 1}, readAdmissionMetrics(t).since(before))
	assert.True(t, h.logs.has("ray cluster was not admitted in time"))

	// Never placed, it is finalized without a call to any target.
	h.reconcileUntilFinalized()
	assertFinalized(t, h.get())
	assert.Equal(t, 1, *enqueues)
}

func TestAdmissionTimeoutAfterAnAmbiguousAttemptRemovesWhatItMayHaveLeft(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, createdAt(scheduledRayCluster(failedLaunch(t, ambiguousRecord))))
	h.onTarget(false)

	h.advance(time.Hour)
	h.reconcile()
	assertAdmissionTimeout(t, h.get(), testAdmissionTimeout)

	// The cluster never launched, but an attempt may have created it.
	h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	h.reconcileUntilFinalized()
	assertFinalized(t, h.get())
}

func TestNoAdmissionTimeout(t *testing.T) {
	tests := []struct {
		name         string
		backend      string
		rc           *v2pb.RayCluster
		kueueManaged bool
		polled       v2pb.RayClusterState // INVALID for a cluster that is not polled
	}{
		{name: "default backend, not placed", backend: "", rc: newRayCluster()},
		{
			name: "default backend, launched and never suspended", backend: "",
			rc: launchedRayCluster(v2pb.RAY_CLUSTER_STATE_UNKNOWN), polled: v2pb.RAY_CLUSTER_STATE_READY,
		},
		{
			name: "kueue backend, admitted", backend: backendKueue,
			rc: launchedRayCluster(v2pb.RAY_CLUSTER_STATE_READY, admitted()), kueueManaged: true, polled: v2pb.RAY_CLUSTER_STATE_READY,
		},
		{
			name: "kueue backend, launched on a target that is not Kueue-managed", backend: backendKueue,
			rc: launchedRayCluster(v2pb.RAY_CLUSTER_STATE_UNKNOWN), polled: v2pb.RAY_CLUSTER_STATE_UNKNOWN,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdmissionHarness(t, tc.backend, tc.rc)
			h.onTarget(tc.kueueManaged)
			if tc.polled != v2pb.RAY_CLUSTER_STATE_INVALID {
				h.pollsReport(tc.polled)
			}
			before := readAdmissionMetrics(t)

			h.advance(10 * testAdmissionTimeout)
			h.reconcile()
			assertNotFailed(t, h.get())
			assert.Zero(t, readAdmissionMetrics(t).since(before).timeouts)
		})
	}
}

func TestReplacementDoesNotRestartTheAdmissionWait(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, createdAt(scheduledRayCluster(failedLaunch(t, windowOver))))
	h.onTarget(false)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewServiceUnavailable("unavailable"))

	// An hour after its creation, the cluster's dispatch window ends and it
	// is re-placed.
	h.reconcile()
	require.Nil(t, h.get().Status.Assignment, "the cluster is re-placed")

	h.advance(time.Hour - time.Second)
	h.reconcile()
	assertNotFailed(t, h.get())

	// Its wait still counts from its creation.
	h.advance(time.Second)
	h.reconcile()
	assertAdmissionTimeout(t, h.get(), testAdmissionTimeout)
}

func TestEnqueueUnderTheKueueBackendWritesNoEnqueued(t *testing.T) {
	tests := []struct {
		name       string
		conditions []*apipb.Condition
	}{
		{name: "a new cluster"},
		{
			name: "after a failed validation",
			conditions: []*apipb.Condition{{
				Type: EnqueuedCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.KueueQueueNotFound,
				LastUpdatedTimestamp: unwrittenTimestamp,
			}},
		},
		{
			name: "after a re-placement",
			conditions: []*apipb.Condition{
				{Type: EnqueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AddedToSchedulerQueue, LastUpdatedTimestamp: unwrittenTimestamp},
				{Type: ScheduledCondition, Status: apipb.CONDITION_STATUS_UNKNOWN, Message: "re-placing from cluster-1: window expired"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdmissionHarness(t, backendKueue, newRayCluster(tc.conditions...))
			enqueues := h.countEnqueues()
			want := jobsutils.FindCondition(tc.conditions, EnqueuedCondition)

			for range 2 {
				assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			}
			assert.Equal(t, 2, *enqueues, "a cluster that is not scheduled is enqueued on every reconcile")
			assert.Zero(t, h.statusWrites)
			enqueued := jobsutils.FindCondition(h.get().Status.StatusConditions, EnqueuedCondition)
			assert.Equal(t, want.GetStatus(), enqueued.GetStatus())
			assert.Equal(t, want.GetReason(), enqueued.GetReason())
			assert.Equal(t, want.GetLastUpdatedTimestamp(), enqueued.GetLastUpdatedTimestamp())
		})
	}
}

func TestEnqueueUnderTheDefaultBackendWritesEnqueued(t *testing.T) {
	h := newAdmissionHarness(t, "", newRayCluster())
	enqueues := h.countEnqueues()

	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	enqueued := jobsutils.FindCondition(h.get().Status.StatusConditions, EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
	assert.Equal(t, 1, h.statusWrites)
	assert.Equal(t, 1, *enqueues)
}

func TestAdmissionWaitIsObservedOncePerAdmission(t *testing.T) {
	tests := []struct {
		name         string
		backend      string
		kueueManaged bool
		atLaunch     admissionMetrics
		total        admissionMetrics
	}{
		{
			name: "kueue backend: a launch on a target that is not Kueue-managed admits", backend: backendKueue,
			atLaunch: admissionMetrics{waits: 1, waitSum: 3600}, total: admissionMetrics{waits: 1, waitSum: 3600},
		},
		{
			name: "kueue backend: Kueue admits on a Kueue-managed target", backend: backendKueue, kueueManaged: true,
			total: admissionMetrics{waits: 1, waitSum: 3660},
		},
		{name: "default backend: a launch does not admit", backend: ""},
		{
			name: "default backend: Kueue admits on a Kueue-managed target", backend: "", kueueManaged: true,
			total: admissionMetrics{waits: 1, waitSum: 3660},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdmissionHarness(t, tc.backend, createdAt(scheduledRayCluster(nil)))
			h.onTarget(tc.kueueManaged)
			h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			h.pollsReport(v2pb.RAY_CLUSTER_STATE_UNKNOWN)
			before := readAdmissionMetrics(t)

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			require.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
			assert.Equal(t, tc.atLaunch, readAdmissionMetrics(t).since(before))

			// The polls after the launch see the cluster unsuspended.
			for range 2 {
				h.advance(time.Minute)
				h.reconcile()
			}
			assert.Equal(t, tc.total, readAdmissionMetrics(t).since(before))
		})
	}
}

func TestAdmissionWaitIsObservedForALaunchWhoseWriteCommittedThenFailed(t *testing.T) {
	h := newAdmissionHarness(t, backendKueue, createdAt(scheduledRayCluster(nil)))
	h.onTarget(false)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	// The Launched=True write commits, but its response is lost.
	h.onStatusWrite = func(ctx context.Context, c client.Client, obj client.Object) error {
		require.NoError(t, c.Status().Update(ctx, obj))
		return errStatusTimeout
	}
	before := readAdmissionMetrics(t)

	_, err := h.reconcileErr()
	require.Error(t, err)
	require.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
	assert.Equal(t, admissionMetrics{waits: 1, waitSum: 3600}, readAdmissionMetrics(t).since(before))

	h.pollsReport(v2pb.RAY_CLUSTER_STATE_UNKNOWN)
	h.advance(time.Minute)
	h.reconcile()
	assert.Equal(t, admissionMetrics{waits: 1, waitSum: 3600}, readAdmissionMetrics(t).since(before))
}

func TestAdmissionIsObservedOnceItsWriteSucceeds(t *testing.T) {
	waitStart := dispatchEpoch.Add(-10 * time.Minute)
	h := newAdmissionHarness(t, backendKueue, launchedRayCluster(v2pb.RAY_CLUSTER_STATE_SUSPENDED, queuedSince(t, waitStart)))
	h.onTarget(true)
	h.pollsReport(v2pb.RAY_CLUSTER_STATE_UNKNOWN)
	h.onStatusWrite = func(context.Context, client.Client, client.Object) error { return errStatusTimeout }
	before := readAdmissionMetrics(t)

	_, err := h.reconcileErr()
	require.Error(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, h.queued().GetStatus())
	assert.Equal(t, admissionMetrics{}, readAdmissionMetrics(t).since(before))

	h.reconcile()
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, h.queued().GetStatus())
	assert.Equal(t, admissionMetrics{waits: 1, waitSum: 600}, readAdmissionMetrics(t).since(before))
}

func TestAdmissionTimeoutIsNotWrittenFromAStaleRead(t *testing.T) {
	tests := map[string]*v2pb.RayCluster{
		"launched since":   launchedRayCluster(v2pb.RAY_CLUSTER_STATE_UNKNOWN),
		"terminated since": createdAt(terminatingRayCluster(failedLaunch(t, windowOver))),
	}
	for name, live := range tests {
		t.Run(name, func(t *testing.T) {
			h := newAdmissionHarness(t, backendKueue, live)
			// The reconciler's read is from before the launch or the failure.
			stale := createdAt(scheduledRayCluster(nil))
			before := readAdmissionMetrics(t)
			h.advance(time.Hour)

			expired, err := h.r.failIfAdmissionExpired(h.ctx, stale, logr.Discard())
			require.NoError(t, err)
			assert.True(t, expired, "the stale read is past the bound")
			assert.Zero(t, h.statusWrites)
			assert.Zero(t, readAdmissionMetrics(t).since(before).timeouts)
			assert.NotEqual(t, constants.AdmissionTimeout, jobsutils.FindCondition(h.get().Status.StatusConditions, SucceededCondition).GetReason())
		})
	}
}
