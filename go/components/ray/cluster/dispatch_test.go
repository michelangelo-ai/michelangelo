package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	dispatchTarget   = assignedCluster
	rayClusterUID    = types.UID("rc-uid")
	clusterNamespace = "ma-system"

	createRetriesMetric = "michelangelo_jobs_create_retries_total"
	replacementsMetric  = "michelangelo_jobs_replacements_total"
)

var (
	dispatchEpoch = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rayClusterKey = types.NamespacedName{Name: rayClusterName, Namespace: testNamespace}

	// windowOver records failed attempts over a whole window.
	windowOver = jobsutils.DispatchState{
		WindowStart: dispatchEpoch.Add(-jobsutils.DispatchWindow), Attempts: 6, NextAttemptAt: dispatchEpoch,
	}
	// ambiguousRecord records an attempt that may have created the cluster.
	ambiguousRecord = jobsutils.DispatchState{
		WindowStart: dispatchEpoch.Add(-time.Minute), Attempts: 2, Ambiguous: true, NextAttemptAt: dispatchEpoch,
	}

	// errStatusTimeout is what a status write that may have committed returns.
	errStatusTimeout = apiErrors.NewTimeoutError("request did not complete within the allowed duration", 0)

	rayClustersResource = schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}
	// errClusterExists is what a create that finds a cluster of the same name returns.
	errClusterExists = apiErrors.NewAlreadyExists(rayClustersResource, rayClusterName)
	// ownClusterOwner is the owner that the job's compute-side cluster records.
	ownClusterOwner = jobsclient.ComputeOwner{Owner: testNamespace + "/" + rayClusterName, UID: string(rayClusterUID)}

	// dispatchBackends are the scheduler backends the owner check runs under.
	dispatchBackends = []struct{ name, backend string }{
		{name: "default backend", backend: ""},
		{name: "kueue backend", backend: backendKueue},
	}
)

// dispatchHarness runs the reconciler's dispatch and cleanup paths against a
// fake API server, under a fake clock. The reconciler's status writes are
// counted, and one can be replaced; the test's own writes go to the server
// directly. Live reads can fail, or see a newer resourceVersion than the
// reconciler's read.
type dispatchHarness struct {
	t         *testing.T
	ctx       context.Context
	server    client.WithWatch
	fed       *clientmocks.MockFederatedClient
	cache     *mockClusterCache
	readiness map[string]jobscluster.Readiness
	live      *liveReader
	logs      *logLines
	r         *Reconciler
	now       time.Time

	statusWrites    int
	readinessChecks int
	// onStatusWrite, when set, replaces the next status write.
	onStatusWrite func(ctx context.Context, c client.Client, obj client.Object) error
}

func newDispatchHarness(t *testing.T, backend string, objects ...client.Object) *dispatchHarness {
	t.Helper()
	jobsclient.RegisterDispatchMetrics()
	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, v2pb.AddToScheme(scheme))

	h := &dispatchHarness{
		t:         t,
		now:       dispatchEpoch,
		cache:     newMockClusterCache(),
		readiness: map[string]jobscluster.Readiness{},
		logs:      &logLines{},
	}
	h.ctx = log.IntoContext(context.Background(), funcr.New(h.logs.add, funcr.Options{}))
	h.server = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(objects...).Build()
	counted := interceptor.NewClient(h.server, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			h.statusWrites++
			if write := h.onStatusWrite; write != nil {
				h.onStatusWrite = nil
				return write(ctx, c, obj)
			}
			return c.SubResource(subResource).Update(ctx, obj, opts...)
		},
	})
	h.live = &liveReader{Reader: h.server}

	mockCtrl := gomock.NewController(t)
	h.fed = clientmocks.NewMockFederatedClient(mockCtrl)
	readiness := clustermocks.NewMockClusterReadiness(mockCtrl)
	readiness.EXPECT().GetClusterReadiness(gomock.Any()).DoAndReturn(func(name string) jobscluster.Readiness {
		h.readinessChecks++
		if r, ok := h.readiness[name]; ok {
			return r
		}
		return jobscluster.ReadinessNotRegistered
	}).AnyTimes()

	h.r = &Reconciler{
		Handler:          &mockAPIHandler{Client: counted},
		federatedClient:  h.fed,
		clusterCache:     h.cache,
		schedulerQueue:   &mockSchedulerQueue{},
		schedulerConfig:  maconfig.SchedulerConfig{Backend: backend},
		clusterReadiness: readiness,
		apiReader:        h.live,
		clusterReader:    h.server,
		now:              func() time.Time { return h.now },
	}
	return h
}

func (h *dispatchHarness) reconcileErr() (ctrl.Result, error) {
	return h.r.Reconcile(h.ctx, ctrl.Request{NamespacedName: rayClusterKey})
}

func (h *dispatchHarness) reconcile() ctrl.Result {
	h.t.Helper()
	res, err := h.reconcileErr()
	require.NoError(h.t, err)
	return res
}

func (h *dispatchHarness) advance(d time.Duration) {
	h.now = h.now.Add(d)
}

func (h *dispatchHarness) get() *v2pb.RayCluster {
	h.t.Helper()
	rc := &v2pb.RayCluster{}
	require.NoError(h.t, h.server.Get(h.ctx, rayClusterKey, rc))
	return rc
}

// state returns the dispatch record in the stored Launched condition.
func (h *dispatchHarness) state() jobsutils.DispatchState {
	h.t.Helper()
	state, err := jobsutils.DispatchStateFromCondition(jobsutils.FindCondition(h.get().Status.StatusConditions, LaunchedCondition))
	require.NoError(h.t, err)
	return state
}

// update writes the RayCluster's status on the server, as another writer would.
func (h *dispatchHarness) update(mutate func(*v2pb.RayCluster)) {
	h.t.Helper()
	rc := h.get()
	mutate(rc)
	require.NoError(h.t, h.server.Status().Update(h.ctx, rc))
}

// placeOn schedules the job on target, as the scheduler would.
func (h *dispatchHarness) placeOn(target string) {
	h.update(func(rc *v2pb.RayCluster) {
		rc.Status.Assignment = &v2pb.AssignmentInfo{Cluster: target}
		scheduled := jobsutils.GetCondition(&rc.Status.StatusConditions, ScheduledCondition, rc.Generation)
		jobsutils.UpdateCondition(scheduled, jobsutils.ConditionUpdateParams{Status: apipb.CONDITION_STATUS_TRUE})
	})
}

// replacedElsewhere takes the job off its target, as a concurrent
// re-placement would.
func (h *dispatchHarness) replacedElsewhere() {
	h.update(func(rc *v2pb.RayCluster) {
		rc.Status.Assignment = nil
		scheduled := jobsutils.GetCondition(&rc.Status.StatusConditions, ScheduledCondition, rc.Generation)
		jobsutils.UpdateCondition(scheduled, jobsutils.ConditionUpdateParams{Status: apipb.CONDITION_STATUS_UNKNOWN})
	})
}

// setReadiness sets a target's readiness. A registered target is in the
// cluster cache.
func (h *dispatchHarness) setReadiness(name string, readiness jobscluster.Readiness) {
	h.readiness[name] = readiness
	if readiness == jobscluster.ReadinessNotRegistered {
		delete(h.cache.clusters, name)
		return
	}
	h.cache.addCluster(name, registeredCluster(name))
}

// liveReader reads the API server directly, as the manager's APIReader does.
type liveReader struct {
	client.Reader
	err  error  // fails every Get when set
	rv   string // reported as the live resourceVersion when set
	gets int
}

func (l *liveReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	l.gets++
	if l.err != nil {
		return l.err
	}
	if err := l.Reader.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if l.rv != "" {
		obj.SetResourceVersion(l.rv)
	}
	return nil
}

// logLines collects formatted log lines.
type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(_, args string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, args)
}

// has reports whether a line contains all of substrings.
func (l *logLines) has(substrings ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		matched := true
		for _, s := range substrings {
			matched = matched && strings.Contains(line, s)
		}
		if matched {
			return true
		}
	}
	return false
}

// scheduledRayCluster is a RayCluster placed on dispatchTarget, with the given
// Launched condition, or none.
func scheduledRayCluster(launched *apipb.Condition) *v2pb.RayCluster {
	conditions := []*apipb.Condition{
		{Type: EnqueuedCondition, Status: apipb.CONDITION_STATUS_TRUE},
		{Type: ScheduledCondition, Status: apipb.CONDITION_STATUS_TRUE},
	}
	if launched != nil {
		conditions = append(conditions, launched)
	}
	return &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: rayClusterName, Namespace: testNamespace, Generation: 1, UID: rayClusterUID},
		Spec:       createRayClusterSpec(),
		Status: v2pb.RayClusterStatus{
			StatusConditions: conditions,
			Assignment:       &v2pb.AssignmentInfo{Cluster: dispatchTarget},
		},
	}
}

// terminatingRayCluster is a RayCluster that failed before it launched on
// dispatchTarget, and is being killed.
func terminatingRayCluster(launched *apipb.Condition) *v2pb.RayCluster {
	rc := scheduledRayCluster(launched)
	rc.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
	rc.Status.StatusConditions = append(rc.Status.StatusConditions,
		&apipb.Condition{Type: SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: clusterCreationFailedReason},
		&apipb.Condition{Type: KillingCondition, Status: apipb.CONDITION_STATUS_TRUE},
	)
	return rc
}

// failedLaunch is a Launched=False condition that records state.
func failedLaunch(t *testing.T, state jobsutils.DispatchState) *apipb.Condition {
	t.Helper()
	metadata, err := state.Metadata()
	require.NoError(t, err)
	return &apipb.Condition{
		Type:     LaunchedCondition,
		Status:   apipb.CONDITION_STATUS_FALSE,
		Reason:   clusterCreationFailedReason,
		Metadata: metadata,
	}
}

func registeredCluster(name string) *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: clusterNamespace}}
}

func launchedStatus(rc *v2pb.RayCluster) apipb.ConditionStatus {
	return jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition).GetStatus()
}

// assertReplaced checks that the job went back to the scheduler with the
// given dispatch record.
func assertReplaced(t *testing.T, rc *v2pb.RayCluster, why string, want jobsutils.DispatchState) {
	t.Helper()
	assert.Nil(t, rc.Status.Assignment)
	scheduled := jobsutils.FindCondition(rc.Status.StatusConditions, ScheduledCondition)
	require.NotNil(t, scheduled)
	assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, scheduled.Status)
	assert.Empty(t, scheduled.Reason)
	assert.Equal(t, "re-placing from cluster-1: "+why, scheduled.Message)
	launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, launched.Status)
	assert.Empty(t, launched.Reason)
	state, err := jobsutils.DispatchStateFromCondition(launched)
	require.NoError(t, err)
	assert.Equal(t, want, state)
}

func assertFinalized(t *testing.T, rc *v2pb.RayCluster) {
	t.Helper()
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_TERMINATED, rc.Status.State)
	assert.True(t, utils.IsImmutable(rc), "the ray cluster should be finalized")
}

// dispatchCounter reads a dispatch counter from the controller-runtime registry.
func dispatchCounter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := metrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			matched := len(metric.GetLabel()) == len(labels)
			for _, label := range metric.GetLabel() {
				matched = matched && labels[label.GetName()] == label.GetValue()
			}
			if matched {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func createRetries(t *testing.T, class jobsclient.DispatchClass) float64 {
	return dispatchCounter(t, createRetriesMetric, map[string]string{"job_type": dispatchMetricsJobType, "class": class.String()})
}

func replacements(t *testing.T, reason string) float64 {
	return dispatchCounter(t, replacementsMetric, map[string]string{"job_type": dispatchMetricsJobType, "reason": reason})
}

func TestDecideAfterFailedAttempt(t *testing.T) {
	now := dispatchEpoch
	open := jobsutils.DispatchState{WindowStart: now.Add(-time.Minute), Attempts: 2}
	over := jobsutils.DispatchState{WindowStart: now.Add(-jobsutils.DispatchWindow), Attempts: 7}
	ambiguous := func(s jobsutils.DispatchState) jobsutils.DispatchState {
		s.Ambiguous = true
		return s
	}

	tests := []struct {
		name        string
		kueue       bool
		state       jobsutils.DispatchState
		targetReady bool
		want        dispatchDecision
	}{
		{name: "kueue: retries within the window", kueue: true, state: open, targetReady: true, want: decisionRetry},
		{name: "kueue: re-places off a target that is not Ready", kueue: true, state: open, want: decisionReplace},
		{name: "kueue: re-places at the window end", kueue: true, state: over, targetReady: true, want: decisionReplace},
		{name: "kueue: retries an ambiguous job off a target that is not Ready", kueue: true, state: ambiguous(open), want: decisionRetry},
		{name: "kueue: fails an ambiguous job at the window end", kueue: true, state: ambiguous(over), want: decisionFail},
		{name: "default: retries off a target that is not Ready", state: open, want: decisionRetry},
		{name: "default: fails at the window end", state: over, targetReady: true, want: decisionFail},
		{name: "default: fails at the window end off a target that is not Ready", state: over, want: decisionFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decideAfterFailedAttempt(tc.kueue, tc.state, tc.targetReady, now))
		})
	}
}

func TestNewReconcilerOptions(t *testing.T) {
	r := NewReconciler(logr.Discard(), nil, env.Context{}, &mockSchedulerQueue{}, nil, newMockClusterCache())
	assert.Equal(t, maconfig.SchedulerConfig{}, r.schedulerConfig)
	assert.Nil(t, r.clusterReadiness)
	assert.False(t, r.kueueBackend())

	readiness := clustermocks.NewMockClusterReadiness(gomock.NewController(t))
	r = NewReconciler(logr.Discard(), nil, env.Context{}, &mockSchedulerQueue{}, nil, newMockClusterCache(),
		WithSchedulerConfig(maconfig.SchedulerConfig{Backend: backendKueue}), WithClusterReadiness(readiness))
	assert.True(t, r.kueueBackend())
	assert.Equal(t, jobscluster.ClusterReadiness(readiness), r.clusterReadiness)
}

func TestDispatchGateOrder(t *testing.T) {
	// An attempt failed a minute ago; its backoff runs 7 seconds more.
	h := newDispatchHarness(t, "", scheduledRayCluster(failedLaunch(t, jobsutils.DispatchState{
		WindowStart: dispatchEpoch.Add(-time.Minute), Attempts: 1, NextAttemptAt: dispatchEpoch.Add(7 * time.Second),
	})))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessUnknown)
	h.live.rv = "1000" // the reconciler's read is behind the live object

	// Gate 1: nothing else is consulted during the backoff.
	assert.Equal(t, ctrl.Result{RequeueAfter: 7 * time.Second}, h.reconcile())
	assert.Zero(t, h.readinessChecks)
	assert.Zero(t, h.live.gets)

	// Gate 2a: an unknown target is waited for before the live read.
	h.advance(7 * time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: unknownTargetRequeue}, h.reconcile())
	assert.Zero(t, h.live.gets)

	// Gate 3: a stale read is not acted on, even for a target that is not Ready.
	h.setReadiness(dispatchTarget, jobscluster.ReadinessNotReady)
	assert.Equal(t, ctrl.Result{RequeueAfter: staleCacheBackoffBase}, h.reconcile())
	assert.Zero(t, h.statusWrites)
	assert.Equal(t, 1, h.state().Attempts)

	// Gate 2b: once the read is fresh, a target that is not Ready gets an
	// unreached attempt, without a dial.
	h.live.rv = ""
	assert.Equal(t, ctrl.Result{RequeueAfter: jobsutils.DispatchBackoff(2)}, h.reconcile())
	assert.Equal(t, 1, h.statusWrites)
	assert.Equal(t, 2, h.state().Attempts)

	// The create, once the target is Ready.
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.advance(jobsutils.DispatchBackoff(2))
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
}

func TestDispatchRetriesATransientFailureInPlace(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	before := createRetries(t, jobsclient.DispatchRejected)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewServiceUnavailable("apiserver restarting"))

	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	rc := h.get()
	assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
	launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
	assert.Equal(t, clusterCreationFailedReason, launched.Reason)
	assert.Equal(t, "apiserver restarting", launched.Message)
	assert.Equal(t, jobsutils.DispatchState{
		WindowStart: dispatchEpoch, Attempts: 1, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
	}, h.state())
	assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchRejected))

	// Gate 1: the reconcile triggered by that write neither dials nor writes.
	h.advance(2 * time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: 3 * time.Second}, h.reconcile())
	assert.Equal(t, 1, h.statusWrites)
}

func TestDispatchLaunchesOnceTheWebhookIsBack(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	webhookDown := apiErrors.NewInternalError(errors.New(`failed calling webhook "mraycluster.kb.io": connection refused`))
	gomock.InOrder(
		h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(webhookDown),
		h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	h.advance(5 * time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())

	rc := h.get()
	assert.Equal(t, v2pb.RAY_CLUSTER_STATE_PROVISIONING, rc.Status.State)
	launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
	assert.Equal(t, clusterCreatedReason, launched.Reason)
	assert.Nil(t, launched.Metadata, "a launch clears the dispatch record")
}

func TestDispatchAlreadyExistsAdoptsTheJobsOwnOrAnUnownedCluster(t *testing.T) {
	owners := []struct {
		name  string
		owner jobsclient.ComputeOwner
	}{
		{name: "the job's", owner: ownClusterOwner},
		{name: "no owner, as on clusters created before owners were recorded"},
	}
	for _, b := range dispatchBackends {
		for _, o := range owners {
			t.Run(b.name+": "+o.name, func(t *testing.T) {
				h := newDispatchHarness(t, b.backend, scheduledRayCluster(nil))
				h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
				gomock.InOrder(
					h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
					h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), registeredCluster(dispatchTarget)).Return(o.owner, nil),
				)

				assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
				rc := h.get()
				assert.Equal(t, v2pb.RAY_CLUSTER_STATE_PROVISIONING, rc.Status.State)
				assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
				launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
				require.NotNil(t, launched)
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
				assert.Equal(t, clusterCreatedReason, launched.Reason)
			})
		}
	}
}

func TestDispatchAlreadyExistsFailsAtOnceOnAnotherJobsCluster(t *testing.T) {
	for _, b := range dispatchBackends {
		t.Run(b.name, func(t *testing.T) {
			h := newDispatchHarness(t, b.backend, scheduledRayCluster(nil))
			h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
			// ns2/test-cluster maps to the same compute-side name. Any further
			// create, and any delete, is an unexpected call.
			theirs := jobsclient.ComputeOwner{Owner: "ns2/" + rayClusterName, UID: "other-uid"}
			gomock.InOrder(
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
				h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), registeredCluster(dispatchTarget)).Return(theirs, nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			rc := h.get()
			assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster(), "the job is not re-placed")
			succeeded := jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, clusterCreationFailedReason, succeeded.Reason)
			assert.Equal(t, `an object of the same name on the compute cluster is owned by another job "ns2/test-cluster" (uid other-uid); `+
				"compute-side names omit the namespace, so rename this job", succeeded.Message)
			assert.NotEqual(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(rc))
			assert.True(t, h.state().IsZero(), "no attempt is recorded")

			// The failed job is killed and finalized without another create
			// or any delete.
			h.advance(time.Minute)
			for i := 0; i < 3 && !utils.IsImmutable(h.get()); i++ {
				h.reconcile()
			}
			assertFinalized(t, h.get())
		})
	}
}

func TestDispatchAlreadyExistsRetriesInPlaceWhenTheOwnerCannotBeRead(t *testing.T) {
	for _, b := range dispatchBackends {
		t.Run(b.name, func(t *testing.T) {
			h := newDispatchHarness(t, b.backend, scheduledRayCluster(nil))
			h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
			before := createRetries(t, jobsclient.DispatchAmbiguous)
			readErr := fmt.Errorf("get ray cluster err:%w", apiErrors.NewServiceUnavailable("unavailable"))
			gomock.InOrder(
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
				h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(jobsclient.ComputeOwner{}, readErr),
				// The retry meets the cluster again, and checks its owner again.
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
				h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(ownClusterOwner, nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
			rc := h.get()
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
			launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
			require.NotNil(t, launched)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
			assert.Equal(t, clusterCreationFailedReason, launched.Reason)
			assert.Equal(t, "an object of the same name exists and its owner could not be read: get ray cluster err:unavailable", launched.Message)
			assert.Equal(t, jobsutils.DispatchState{
				WindowStart: dispatchEpoch, Attempts: 1, Ambiguous: true, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
			}, h.state())
			assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchAmbiguous))

			h.advance(5 * time.Second)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
		})
	}
}

func TestDispatchAlreadyExistsRetriesTheCreateWhenTheClusterIsGone(t *testing.T) {
	for _, b := range dispatchBackends {
		t.Run(b.name, func(t *testing.T) {
			h := newDispatchHarness(t, b.backend, scheduledRayCluster(nil))
			h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
			before := createRetries(t, jobsclient.DispatchRejected)
			gone := fmt.Errorf("get ray cluster err:%w", apiErrors.NewNotFound(rayClustersResource, rayClusterName))
			gomock.InOrder(
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
				h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(jobsclient.ComputeOwner{}, gone),
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
			rc := h.get()
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
			launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
			require.NotNil(t, launched)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
			assert.Equal(t, jobsutils.DispatchState{
				WindowStart: dispatchEpoch, Attempts: 1, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
			}, h.state(), "a create that created nothing is not ambiguous")
			assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchRejected))

			h.advance(5 * time.Second)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
		})
	}
}

func TestDispatchWaitsForAnUnknownTarget(t *testing.T) {
	tests := []struct {
		name      string
		readiness jobscluster.Readiness
		objects   []client.Object
	}{
		{name: "registered but never probed", readiness: jobscluster.ReadinessUnknown},
		{
			// The cluster cache has not caught up, so getClusterIfScheduled
			// finds nothing.
			name:      "Cluster object not yet cached",
			readiness: jobscluster.ReadinessNotRegistered,
			objects:   []client.Object{registeredCluster(dispatchTarget)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDispatchHarness(t, backendKueue, append(tc.objects, scheduledRayCluster(nil))...)
			h.setReadiness(dispatchTarget, tc.readiness)

			assert.Equal(t, ctrl.Result{RequeueAfter: unknownTargetRequeue}, h.reconcile())
			assert.Zero(t, h.statusWrites)
			rc := h.get()
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
			assert.Nil(t, jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition), "no attempt is counted")
		})
	}
}

func TestDispatchToADeregisteredTarget(t *testing.T) {
	deleting := registeredCluster(dispatchTarget)
	deleting.DeletionTimestamp = &metav1.Time{Time: dispatchEpoch}
	deleting.Finalizers = []string{"test/hold"}

	for name, objects := range map[string][]client.Object{"absent": nil, "being deleted": {deleting}} {
		t.Run("kueue re-places: "+name, func(t *testing.T) {
			h := newDispatchHarness(t, backendKueue, append(objects, scheduledRayCluster(nil))...)
			before := replacements(t, jobsclient.ReplacementReasonTargetNotReady)

			assert.Equal(t, ctrl.Result{}, h.reconcile())
			assertReplaced(t, h.get(), "target not ready", jobsutils.DispatchState{
				Replacements: 1, NextAttemptAt: dispatchEpoch.Add(10 * time.Second), ExcludedClusters: []string{dispatchTarget},
			})
			assert.Equal(t, before+1, replacements(t, jobsclient.ReplacementReasonTargetNotReady))
		})
	}
	t.Run("default backend requeues", func(t *testing.T) {
		h := newDispatchHarness(t, "", scheduledRayCluster(nil))

		assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
		assert.Zero(t, h.statusWrites)
		assert.Equal(t, dispatchTarget, h.get().Status.Assignment.GetCluster())
	})
}

func TestDispatchRecordsAnUnreachedAttemptOnATargetThatIsNotReady(t *testing.T) {
	h := newDispatchHarness(t, "", scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessNotReady)
	before := createRetries(t, jobsclient.DispatchUnreached)

	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	rc := h.get()
	assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
	launched := jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
	assert.Equal(t, `target cluster "cluster-1" is not Ready`, launched.Message)
	assert.Equal(t, jobsutils.DispatchState{
		WindowStart: dispatchEpoch, Attempts: 1, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
	}, h.state())
	assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchUnreached))
}

func TestDispatchReplacesACleanJobAtTheWindowEnd(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(failedLaunch(t, windowOver)))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewServiceUnavailable("unavailable"))
	before := replacements(t, jobsclient.ReplacementReasonWindowExpired)

	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertReplaced(t, h.get(), "window expired", jobsutils.DispatchState{
		Replacements: 1, NextAttemptAt: dispatchEpoch.Add(10 * time.Second), ExcludedClusters: []string{dispatchTarget},
	})
	assert.Equal(t, before+1, replacements(t, jobsclient.ReplacementReasonWindowExpired))
}

func TestDispatchFailsAnAmbiguousJobAtTheWindowEnd(t *testing.T) {
	earlierAmbiguous := windowOver
	earlierAmbiguous.Ambiguous = true
	tests := []struct {
		name   string
		record jobsutils.DispatchState
		err    error
	}{
		{name: "an earlier attempt was ambiguous", record: earlierAmbiguous, err: apiErrors.NewServiceUnavailable("unavailable")},
		{name: "the last attempt is ambiguous", record: windowOver, err: errStatusTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDispatchHarness(t, backendKueue, scheduledRayCluster(failedLaunch(t, tc.record)))
			h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
			h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.err)

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			rc := h.get()
			assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster(), "an ambiguous job is not re-placed")
			succeeded := jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, clusterCreationFailedReason, succeeded.Reason)
			assert.Equal(t, "could not create the ray cluster on cluster-1 in 7 attempts over 10m0s: "+tc.err.Error(), succeeded.Message)
			state := h.state()
			assert.True(t, state.Ambiguous, "cleanup must know the cluster may exist")
			assert.Equal(t, 7, state.Attempts)
		})
	}
}

func TestDispatchDefaultBackendNeverReplaces(t *testing.T) {
	tests := []struct {
		name      string
		readiness jobscluster.Readiness
		create    bool
	}{
		{name: "window over", readiness: jobscluster.ReadinessReady, create: true},
		{name: "window over on a target that is not Ready", readiness: jobscluster.ReadinessNotReady},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDispatchHarness(t, "", scheduledRayCluster(failedLaunch(t, windowOver)))
			h.setReadiness(dispatchTarget, tc.readiness)
			if tc.create {
				h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewServiceUnavailable("unavailable"))
			}

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			rc := h.get()
			assert.Equal(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
			assert.Equal(t, dispatchTarget, rc.Status.Assignment.GetCluster())
			assert.Zero(t, h.state().Replacements)
		})
	}
}

func TestDispatchReplacementKeepsTheBackoff(t *testing.T) {
	// The job is pinned to a target that stays not Ready: the scheduler puts
	// it back there after every re-placement.
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessNotReady)

	var replacedAt []time.Time
	for len(replacedAt) < 5 {
		require.Less(t, h.now.Sub(dispatchEpoch), 10*time.Minute, "re-placements stopped")
		h.reconcile()
		if h.get().Status.Assignment == nil {
			replacedAt = append(replacedAt, h.now)
			h.placeOn(dispatchTarget)
		}
		h.advance(time.Second)
	}

	var intervals []time.Duration
	for i := 1; i < len(replacedAt); i++ {
		intervals = append(intervals, replacedAt[i].Sub(replacedAt[i-1]))
	}
	assert.Equal(t, []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}, intervals)
	assert.Equal(t, len(replacedAt), h.statusWrites, "nothing is written between re-placements")
	assert.Equal(t, 5, h.state().Replacements)
	assert.Equal(t, []string{dispatchTarget}, h.state().ExcludedClusters, "a target left again is excluded once")
}

func TestDispatchWaitsOutAStaleRead(t *testing.T) {
	h := newDispatchHarness(t, "", scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.live.rv = "1000" // the reconciler's read is behind the live object

	var waits []time.Duration
	for i := 0; i < 6; i++ {
		waits = append(waits, h.reconcile().RequeueAfter)
	}
	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second,
	}, waits)
	assert.Zero(t, h.statusWrites)
	assert.True(t, h.logs.has("cache still behind the live ray cluster"))

	// Once the read is fresh the create goes ahead, and the wait starts over.
	h.live.rv = ""
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(apiErrors.NewServiceUnavailable("unavailable"))
	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	h.advance(5 * time.Second)
	h.live.rv = "1001"
	assert.Equal(t, ctrl.Result{RequeueAfter: time.Second}, h.reconcile())
}

func TestDispatchAttemptAfterAConcurrentReplacementWritesNothing(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantDelete bool
	}{
		{name: "rejected", err: apiErrors.NewServiceUnavailable("unavailable")},
		{name: "ambiguous, so the cluster is removed", err: errStatusTimeout, wantDelete: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
			h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
			h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(context.Context, runtime.Object, *v2pb.Cluster) error {
					h.replacedElsewhere()
					return tc.err
				})
			if tc.wantDelete {
				h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			}

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			assert.Zero(t, h.statusWrites)
			rc := h.get()
			assert.Nil(t, rc.Status.Assignment)
			assert.Nil(t, jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition))
		})
	}
}

func TestDispatchWindowEndFailureAfterAConcurrentReplacementWritesNothing(t *testing.T) {
	h := newDispatchHarness(t, "", scheduledRayCluster(failedLaunch(t, windowOver)))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, runtime.Object, *v2pb.Cluster) error {
			h.replacedElsewhere()
			return apiErrors.NewServiceUnavailable("unavailable")
		})

	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	assert.Zero(t, h.statusWrites)
	rc := h.get()
	assert.NotEqual(t, v2pb.RAY_CLUSTER_STATE_FAILED, rc.Status.State)
	assert.Nil(t, jobsutils.FindCondition(rc.Status.StatusConditions, SucceededCondition))
	assert.Equal(t, windowOver, h.state())
}

func TestDispatchSuccessAfterAConcurrentReplacementRemovesTheCluster(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	gomock.InOrder(
		h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, runtime.Object, *v2pb.Cluster) error {
				h.replacedElsewhere()
				return nil
			}),
		h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	assert.Zero(t, h.statusWrites)
	assert.NotEqual(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
}

func TestDispatchLaunchedWriteThatCommitsThenFailsDeletesNothing(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	// The Launched=True write commits, but its response is lost.
	h.onStatusWrite = func(ctx context.Context, c client.Client, obj client.Object) error {
		require.NoError(t, c.Status().Update(ctx, obj))
		return errStatusTimeout
	}

	_, err := h.reconcileErr()
	require.Error(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
	assert.False(t, h.r.isLatched(rayClusterUID))
}

func TestDispatchLaunchedWriteFailureAfterTheJobMovedRemovesTheCluster(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	// The Launched=True write fails, and the job was re-placed meanwhile.
	h.onStatusWrite = func(context.Context, client.Client, client.Object) error {
		h.replacedElsewhere()
		return errStatusTimeout
	}

	_, err := h.reconcileErr()
	require.Error(t, err)
	assert.Nil(t, h.get().Status.Assignment)
	assert.False(t, h.r.isLatched(rayClusterUID))
}

func TestDispatchLaunchedWriteFailureWithoutALiveReadLatches(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, runtime.Object, *v2pb.Cluster) error {
			h.live.err = errors.New("apiserver unreachable")
			return nil
		})
	h.onStatusWrite = func(context.Context, client.Client, client.Object) error { return errStatusTimeout }

	_, err := h.reconcileErr()
	require.Error(t, err)
	assert.True(t, h.r.isLatched(rayClusterUID), "a creation that could not be recorded is kept in memory")
	assert.Nil(t, jobsutils.FindCondition(h.get().Status.StatusConditions, LaunchedCondition))
}

func TestDispatchLaunchedWriteFailureRecordsAPossibleCreation(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	h.onStatusWrite = func(context.Context, client.Client, client.Object) error { return errStatusTimeout }

	_, err := h.reconcileErr()
	require.Error(t, err)
	assert.True(t, h.state().Ambiguous)
	assert.False(t, h.r.isLatched(rayClusterUID))

	// The next create finds the cluster, which records the job as its owner,
	// and adopts it.
	gomock.InOrder(
		h.fed.EXPECT().CreateJobCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(errClusterExists),
		h.fed.EXPECT().GetJobClusterOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(ownClusterOwner, nil),
	)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launchedStatus(h.get()))
}

func TestReplaceWritesNothingOverAPossibleCreationOrALaunch(t *testing.T) {
	tests := map[string]*apipb.Condition{
		"may have been created": failedLaunch(t, ambiguousRecord),
		"launched":              {Type: LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: clusterCreatedReason},
	}
	for name, launched := range tests {
		t.Run(name, func(t *testing.T) {
			h := newDispatchHarness(t, backendKueue, scheduledRayCluster(launched))
			// The reconcile read the job before that record was written.
			read := scheduledRayCluster(nil)
			state := jobsutils.DispatchState{}.WithAttempt(dispatchEpoch, false)

			res, err := h.r.replace(h.ctx, read, state, failedAttempt{
				target: dispatchTarget, class: jobsclient.DispatchUnreached, cause: errors.New("target not ready"), now: dispatchEpoch,
			}, logr.Discard())
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			assert.Zero(t, h.statusWrites)
			assert.Equal(t, dispatchTarget, h.get().Status.Assignment.GetCluster())
		})
	}
}

func TestDispatchDoesNotReplaceALatchedJob(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, scheduledRayCluster(nil))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessNotReady)
	h.r.latch(rayClusterUID)

	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	assert.Equal(t, dispatchTarget, h.get().Status.Assignment.GetCluster())
	assert.True(t, h.state().Ambiguous, "the latch is persisted")
	assert.False(t, h.r.isLatched(rayClusterUID))
}

func TestCleanupFinalizesAtOnceAfterOnlyUnreachedOrRejectedAttempts(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, terminatingRayCluster(failedLaunch(t, windowOver)))
	// Not even an unknown target is waited for.
	h.setReadiness(dispatchTarget, jobscluster.ReadinessUnknown)

	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertFinalized(t, h.get())
	assert.True(t, h.state().CleanupStart.IsZero())
}

func TestCleanupRemovesWhatAnAmbiguousCreateMayHaveLeft(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, terminatingRayCluster(failedLaunch(t, ambiguousRecord)))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertFinalized(t, h.get())
	assert.Equal(t, dispatchEpoch, h.state().CleanupStart)
}

func TestCleanupRemovesALatchedCluster(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, terminatingRayCluster(failedLaunch(t, windowOver)))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.r.latch(rayClusterUID)
	h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertFinalized(t, h.get())
	assert.False(t, h.r.isLatched(rayClusterUID))
}

func TestCleanupWaitsForTheTargetWithinTheWindow(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, terminatingRayCluster(failedLaunch(t, ambiguousRecord)))
	h.setReadiness(dispatchTarget, jobscluster.ReadinessNotReady)

	// Not Ready: the window opens, once, and nothing is dialed.
	assert.Equal(t, ctrl.Result{RequeueAfter: cleanupRequeue}, h.reconcile())
	assert.Equal(t, dispatchEpoch, h.state().CleanupStart)
	assert.Equal(t, 1, h.statusWrites)

	// Unknown, then a failing delete: retried, with no write per retry.
	h.advance(cleanupRequeue)
	h.setReadiness(dispatchTarget, jobscluster.ReadinessUnknown)
	assert.Equal(t, ctrl.Result{RequeueAfter: cleanupRequeue}, h.reconcile())
	h.advance(cleanupRequeue)
	h.setReadiness(dispatchTarget, jobscluster.ReadinessReady)
	h.fed.EXPECT().DeleteJobClusterIfOwned(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("connection refused"))
	assert.Equal(t, ctrl.Result{RequeueAfter: cleanupRequeue}, h.reconcile())
	assert.Equal(t, 1, h.statusWrites)
	assert.False(t, utils.IsImmutable(h.get()))

	// Ten minutes after the window opened: give up without dialing, log the
	// identity, and finalize.
	h.now = dispatchEpoch.Add(jobsutils.DispatchWindow)
	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertFinalized(t, h.get())
	assert.True(t, h.logs.has(`"error"=`, "giving up", string(rayClusterUID)))
}

func TestCleanupFinalizesForADeregisteredTarget(t *testing.T) {
	h := newDispatchHarness(t, backendKueue, terminatingRayCluster(failedLaunch(t, ambiguousRecord)))

	assert.Equal(t, ctrl.Result{}, h.reconcile())
	assertFinalized(t, h.get())
}

func TestCleanupWaitsForAnUnknownTargetAtStartup(t *testing.T) {
	// At startup the cluster cache is empty, while the target's Cluster exists.
	h := newDispatchHarness(t, backendKueue, registeredCluster(dispatchTarget), terminatingRayCluster(failedLaunch(t, ambiguousRecord)))

	assert.Equal(t, ctrl.Result{RequeueAfter: cleanupRequeue}, h.reconcile())
	assert.False(t, utils.IsImmutable(h.get()))
}
