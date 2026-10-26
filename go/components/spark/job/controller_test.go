package job

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"slices"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/controllerutil"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	constants "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	sparkJobName      = "test-spark-job"
	testNamespace     = "default"
	testClusterName   = "member-cluster-1"
	testJobGeneration = 1

	// controllerTestFinalizer keeps a deleted SparkJob readable. The fake client
	// rejects an object created with a deletion timestamp, so a test sets the
	// timestamp with a real Delete of an object this finalizer holds.
	controllerTestFinalizer = "test.michelangelo.ai/controller-hold"

	// unregisteredClusterMessage is the Killed message of a termination whose
	// assigned cluster the control plane no longer knows.
	unregisteredClusterMessage = `the SparkApplication on cluster "member-cluster-1" could not be deleted because that cluster is not registered with the control plane`
)

// mockSchedulerQueue is a test double for scheduler.JobQueue.
//
// Everything enqueued is recorded, and enqueueFunc gives a test a hook that
// runs *during* Enqueue — which is how the ordering guarantee (the Enqueued
// condition is persisted before the job reaches the queue) is observed, and how
// a queue that already holds the job is simulated.
type mockSchedulerQueue struct {
	enqueueFunc func(ctx context.Context, job matypes.SchedulableJob) error
	enqueued    []matypes.SchedulableJob
}

func (m *mockSchedulerQueue) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
	m.enqueued = append(m.enqueued, job)
	if m.enqueueFunc != nil {
		return m.enqueueFunc(ctx, job)
	}
	return nil
}

// reconcileFixture is a Reconciler wired to a fake API server, a recording
// scheduler queue and strict mocks for the federated client and the cluster
// cache: a call nobody declared fails the test, which is how the "does not
// touch the member cluster" expectations below are enforced. What the
// reconciles log, at every level, is recorded in logs.
type reconcileFixture struct {
	r         *Reconciler
	queue     *mockSchedulerQueue
	federated *clientmocks.MockFederatedClient
	cache     *clustermocks.MockRegisteredClustersCache
	logger    logr.Logger
	logs      *observer.ObservedLogs
}

func newReconcileFixture(t *testing.T, objects ...client.Object) *reconcileFixture {
	t.Helper()
	return newReconcileFixtureWith(t, nil, objects...)
}

// newReconcileFixtureWith is newReconcileFixture with funcs intercepting the
// fake API server's calls, for the tests that make a read or a write fail.
func newReconcileFixtureWith(t *testing.T, funcs *interceptor.Funcs, objects ...client.Object) *reconcileFixture {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objects) > 0 {
		// WithStatusSubresource makes Status().Update and the plain Update that
		// marks the job immutable behave as they do in production.
		builder = builder.WithObjects(objects...).WithStatusSubresource(objects...)
	}
	if funcs != nil {
		builder = builder.WithInterceptorFuncs(*funcs)
	}

	mockCtrl := gomock.NewController(t)
	observedCore, logs := observer.New(zapcore.DebugLevel)
	f := &reconcileFixture{
		queue:     &mockSchedulerQueue{},
		federated: clientmocks.NewMockFederatedClient(mockCtrl),
		cache:     clustermocks.NewMockRegisteredClustersCache(mockCtrl),
		logger:    zapr.NewLogger(zap.New(observedCore)),
		logs:      logs,
	}
	f.r = &Reconciler{
		Client:          builder.Build(),
		schedulerQueue:  f.queue,
		federatedClient: f.federated,
		clusterCache:    f.cache,
		// Reconcile never maps a status, so the mapper carries no expectations.
		mapper: typesmocks.NewMockMapper(mockCtrl),
	}
	return f
}

// reconcile reconciles the SparkJob named sparkJobName.
func (f *reconcileFixture) reconcile() (ctrl.Result, error) {
	return f.reconcileNamed(sparkJobName)
}

// reconcileNamed reconciles the SparkJob with the given name, logging to
// f.logs.
func (f *reconcileFixture) reconcileNamed(name string) (ctrl.Result, error) {
	ctx := log.IntoContext(context.Background(), f.logger)
	return f.r.Reconcile(ctx, ctrl.Request{NamespacedName: sparkJobKeyNamed(name)})
}

// stored reads the SparkJob named sparkJobName back from the fake API server.
func (f *reconcileFixture) stored(t *testing.T) *v2pb.SparkJob {
	t.Helper()
	return f.storedNamed(t, sparkJobName)
}

// storedNamed reads the SparkJob with the given name back from the fake API
// server.
func (f *reconcileFixture) storedNamed(t *testing.T, name string) *v2pb.SparkJob {
	t.Helper()
	var job v2pb.SparkJob
	require.NoError(t, f.r.Get(context.Background(), sparkJobKeyNamed(name), &job))
	return &job
}

func sparkJobKeyNamed(name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: testNamespace}
}

// testSparkJob builds a SparkJob carrying the given status conditions.
func testSparkJob(conditions ...*apipb.Condition) *v2pb.SparkJob {
	return &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       sparkJobName,
			Namespace:  testNamespace,
			Generation: testJobGeneration,
		},
		Status: v2pb.SparkJobStatus{StatusConditions: conditions},
	}
}

// scheduledSparkJob builds a SparkJob the scheduler has already assigned to
// testClusterName, optionally with further conditions such as Launched.
func scheduledSparkJob(conditions ...*apipb.Condition) *v2pb.SparkJob {
	job := testSparkJob(append([]*apipb.Condition{
		testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE),
		testCondition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE),
	}, conditions...)...)
	job.Status.Assignment = &v2pb.AssignmentInfo{Cluster: testClusterName}
	return job
}

// launchedSparkJob builds a SparkJob running on testClusterName as the watch
// last recorded it: launched, with Succeeded still UNKNOWN in state RUNNING.
// Every condition was written ten minutes ago, so the job's status has been
// quiet for longer than sparkStatusProbeInterval and a probe is due.
func launchedSparkJob() *v2pb.SparkJob {
	running := testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN)
	running.Reason = "RUNNING"
	job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE), running)
	stampConditions(job, time.Now().Add(-10*time.Minute))
	return job
}

// launchedSparkJobOn builds a SparkJob like launchedSparkJob, with the given
// name and running on the named cluster.
func launchedSparkJobOn(name, cluster string) *v2pb.SparkJob {
	job := launchedSparkJob()
	job.Name = name
	job.Status.Assignment.Cluster = cluster
	return job
}

// stampConditions records every condition of job as last written at the given
// time, which is how the probe tells how long the job's status has been quiet.
func stampConditions(job *v2pb.SparkJob, at time.Time) {
	for _, condition := range job.Status.StatusConditions {
		condition.LastUpdatedTimestamp = at.Unix()
	}
}

// rejectedSparkJob builds a SparkJob the queue's pre-flight rejected for reason
// age ago, as the Kueue decorator records a rejection: Enqueued=FALSE with the
// reason and a message of the kind the decorator writes for it, stamped when it
// was written.
func rejectedSparkJob(reason string, age time.Duration) *v2pb.SparkJob {
	rejected := testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_FALSE)
	rejected.Reason = reason
	rejected.Message = `LocalQueue "test-project" not found in namespace "default" on cluster "member-cluster-1"`
	if reason == constants.KueueQueueCheckFailed {
		rejected.Message = `checking LocalQueue "test-project" on cluster "member-cluster-1": context deadline exceeded`
	}
	rejected.LastUpdatedTimestamp = time.Now().Add(-age).Unix()
	return testSparkJob(rejected)
}

func testCondition(conditionType string, status apipb.ConditionStatus) *apipb.Condition {
	return &apipb.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: testJobGeneration,
	}
}

func testTermination(terminationType v2pb.TerminationType) *v2pb.TerminationSpec {
	return &v2pb.TerminationSpec{Type: terminationType, Reason: "workflow cancelled"}
}

func testCluster() *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: testClusterName}}
}

// registeredCluster is the Cluster object that registers the cluster named
// testClusterName, for the tests in which the cluster cache misses it.
func registeredCluster() *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: constants.ClustersNamespace, Name: testClusterName}}
}

// isRegisteredCluster matches a copy of registeredCluster as read from the
// fake API server, and not testCluster, which is how the cache returns the
// cluster: it tells a delete made through the Cluster object from one made
// through the cache's copy.
type isRegisteredCluster struct{}

func (isRegisteredCluster) Matches(x interface{}) bool {
	cluster, ok := x.(*v2pb.Cluster)
	return ok && cluster.Namespace == constants.ClustersNamespace && cluster.Name == testClusterName
}

func (isRegisteredCluster) String() string {
	return fmt.Sprintf("is the Cluster object %s/%s", constants.ClustersNamespace, testClusterName)
}

// failClusterReads is a Get interceptor that fails every read of a Cluster
// object with err and passes every other read through.
func failClusterReads(err error) func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
	return func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		if _, ok := obj.(*v2pb.Cluster); ok {
			return err
		}
		return c.Get(ctx, key, obj, opts...)
	}
}

// probedSparkStatus builds the status the federated client maps a
// SparkApplication to, with its Succeeded condition in the given state.
func probedSparkStatus(status apipb.ConditionStatus, reason, message string) *matypes.JobStatus {
	return &matypes.JobStatus{Spark: &v2pb.SparkJobStatus{
		ApplicationId: "spark-application-1",
		StatusConditions: []*apipb.Condition{{
			Type:    constants.SucceededCondition,
			Status:  status,
			Reason:  reason,
			Message: message,
		}},
	}}
}

// recordOutcome writes a Succeeded condition onto the stored SparkJob the way
// the watch does, standing in for an event that lands in the middle of a
// reconcile.
func recordOutcome(t *testing.T, f *reconcileFixture, status apipb.ConditionStatus, reason string) {
	t.Helper()
	current := f.stored(t)
	succeeded := jobsutils.GetCondition(&current.Status.StatusConditions, constants.SucceededCondition, current.Generation)
	jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
		Status:     status,
		Reason:     reason,
		Generation: current.Generation,
	})
	require.NoError(t, f.r.Status().Update(context.Background(), current))
}

// ageLaunchedCondition stamps the stored SparkJob's Launched condition as
// written age ago, standing in for the time that passes between two attempts
// to create its SparkApplication.
func ageLaunchedCondition(t *testing.T, f *reconcileFixture, age time.Duration) {
	t.Helper()
	current := f.stored(t)
	launched := findCondition(current.Status.StatusConditions, constants.LaunchedCondition)
	require.NotNil(t, launched, "the job has no Launched condition to age")
	launched.LastUpdatedTimestamp = time.Now().Add(-age).Unix()
	require.NoError(t, f.r.Status().Update(context.Background(), current))
}

// requireRemoteCallDeadline asserts that ctx carries the remote-call deadline,
// remoteCallTimeout: set, at most 30 seconds away, and not some much shorter
// one left over from elsewhere.
func requireRemoteCallDeadline(t *testing.T, ctx context.Context) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	require.True(t, ok, "a call to a member cluster must carry a deadline")
	remaining := time.Until(deadline)
	assert.LessOrEqual(t, remaining, 30*time.Second)
	assert.Greater(t, remaining, 20*time.Second)
}

// TestReconciler_Reconcile_NotFound covers a SparkJob that no longer exists:
// the reconcile is a no-op and must not be retried.
func TestReconciler_Reconcile_NotFound(t *testing.T) {
	f := newReconcileFixture(t)

	res, err := f.reconcile()

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
	assert.Empty(t, f.queue.enqueued, "a SparkJob that does not exist must not be enqueued")
}

// TestReconciler_Reconcile_GetFails covers a read of the SparkJob that fails for
// a reason other than the job being gone: the error is returned for the
// controller's backoff, with no RequeueAfter beside it, which controller-runtime
// would discard anyway, and nothing else happens.
func TestReconciler_Reconcile_GetFails(t *testing.T) {
	injected := errors.New("etcdserver: request timed out")
	f := newReconcileFixtureWith(t, &interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return injected
		},
	}, testSparkJob())

	res, err := f.reconcile()

	require.ErrorIs(t, err, injected)
	assert.Equal(t, ctrl.Result{}, res)
	assert.Empty(t, f.queue.enqueued)
}

// TestReconciler_Reconcile_Enqueue covers the admission path: a SparkJob the
// controller has not seen before goes onto the scheduler's queue, and one that
// says it was enqueued but has never been scheduled goes back on it — the queue
// lives in memory, so a controllermgr restart loses it.
func TestReconciler_Reconcile_Enqueue(t *testing.T) {
	t.Run("fresh job is enqueued and records the condition first", func(t *testing.T) {
		f := newReconcileFixture(t, testSparkJob())

		// Read the stored object back from inside Enqueue: the condition has to
		// be stored before the job is on the queue. The scheduler reads the job
		// through the manager's cache, which may lag behind the write, but its
		// assignment write is conditional on the resourceVersion it read, so it
		// cannot write back a copy from before the mark.
		var condAtEnqueueTime *apipb.Condition
		f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error {
			condAtEnqueueTime = findCondition(f.stored(t).Status.StatusConditions, constants.EnqueuedCondition)
			return nil
		}

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, requeueAfter, res.RequeueAfter)

		require.NotNil(t, condAtEnqueueTime, "Enqueued condition must be persisted before the job is enqueued")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, condAtEnqueueTime.Status)

		require.Len(t, f.queue.enqueued, 1, "the job must be enqueued exactly once")
		enqueued := f.queue.enqueued[0]
		assert.Equal(t, matypes.SparkJob, enqueued.GetJobType())
		assert.Equal(t, sparkJobName, enqueued.GetName())
		assert.Equal(t, testNamespace, enqueued.GetNamespace())
		assert.Equal(t, int64(testJobGeneration), enqueued.GetGeneration())

		enqueuedCond := findCondition(f.stored(t).Status.StatusConditions, constants.EnqueuedCondition)
		require.NotNil(t, enqueuedCond)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueuedCond.Status)
		assert.Equal(t, constants.AddedToSchedulerQueue, enqueuedCond.Reason)
	})

	scheduledNotTrue := map[string]*v2pb.SparkJob{
		"scheduled condition absent": testSparkJob(
			testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE),
		),
		"scheduled condition false": testSparkJob(
			testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE),
			testCondition(constants.ScheduledCondition, apipb.CONDITION_STATUS_FALSE),
		),
	}
	for name, job := range scheduledNotTrue {
		t.Run("already enqueued but unscheduled is re-enqueued without a status write: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, job)
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, requeueAfter, res.RequeueAfter)
			assert.Len(t, f.queue.enqueued, 1, "the job must be enqueued exactly once")
			assert.Equal(t, before, f.stored(t).ResourceVersion, "re-enqueueing must not rewrite status")
		})
	}

	alreadyQueued := map[string]*v2pb.SparkJob{
		"first enqueue": testSparkJob(),
		"re-enqueue": testSparkJob(
			testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE),
		),
	}
	for name, job := range alreadyQueued {
		t.Run("a job already on the queue is not an error: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, job)
			f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error {
				// Wrapped, as Scheduler.Enqueue returns it.
				return fmt.Errorf("enqueue err:%w", matypes.ErrJobAlreadyExists)
			}

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, requeueAfter, res.RequeueAfter)
			assert.Len(t, f.queue.enqueued, 1, "the job must have reached the queue")
		})
	}
}

// TestReconciler_Reconcile_EnqueueFails covers a queue that refuses a job for a
// reason other than already holding it. The error is returned, and the
// job heals on a later reconcile: its Enqueued condition was written before the
// failed Enqueue, so the next tick takes the restart branch and enqueues it
// again.
func TestReconciler_Reconcile_EnqueueFails(t *testing.T) {
	injected := errors.New("scheduler queue is shutting down")

	t.Run("a fresh job is enqueued again on the next reconcile", func(t *testing.T) {
		f := newReconcileFixture(t, testSparkJob())
		f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error { return injected }

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		enqueued := findCondition(f.stored(t).Status.StatusConditions, constants.EnqueuedCondition)
		require.NotNil(t, enqueued)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)

		f.queue.enqueueFunc = nil
		before := f.stored(t).ResourceVersion

		res, err = f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, requeueAfter, res.RequeueAfter)
		assert.Len(t, f.queue.enqueued, 2, "the job must be put back on the queue")
		assert.Equal(t, before, f.stored(t).ResourceVersion, "re-enqueueing must not rewrite status")
	})

	t.Run("an unscheduled job the queue refuses returns the error", func(t *testing.T) {
		f := newReconcileFixture(t, testSparkJob(testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE)))
		f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error { return injected }

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		assert.Len(t, f.queue.enqueued, 1)
	})
}

// TestReconciler_Reconcile_EnqueueAfterRejection covers a job the queue's
// pre-flight rejected and recorded as Enqueued=FALSE. It is retried only
// once the rejection is old enough, so the rejection's own status write, whose
// event reconciles the job at once, does not start a tight loop: requeueAfter
// for a LocalQueue that does not exist, and sparkStatusProbeInterval for a
// check that could not ask the cluster at all, which may have held a worker
// for as long as the check may take. Enqueued=TRUE is written only once the
// queue has accepted the job, so the recorded reason stays put while it has
// not.
func TestReconciler_Reconcile_EnqueueAfterRejection(t *testing.T) {
	recent := map[string]time.Duration{
		"just now":         0,
		"five seconds ago": 5 * time.Second,
	}
	for name, age := range recent {
		t.Run("a recent rejection is not retried yet: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, rejectedSparkJob(constants.KueueQueueNotFound, age))
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			assert.Empty(t, f.queue.enqueued, "a fresh rejection must not be retried")
			stored := f.stored(t)
			assert.Equal(t, before, stored.ResourceVersion, "waiting out a rejection writes nothing")
			enqueued := findCondition(stored.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued.Status)
			assert.Equal(t, constants.KueueQueueNotFound, enqueued.Reason)
		})
	}

	accepted := map[string]error{
		"the queue takes the job":    nil,
		"the queue already holds it": fmt.Errorf("enqueue err:%w", matypes.ErrJobAlreadyExists),
	}
	for name, enqueueErr := range accepted {
		t.Run("an old rejection is retried and TRUE is written once the queue accepts the job: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, rejectedSparkJob(constants.KueueQueueNotFound, 30*time.Second))

			var condAtEnqueueTime *apipb.Condition
			f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error {
				condAtEnqueueTime = findCondition(f.stored(t).Status.StatusConditions, constants.EnqueuedCondition)
				return enqueueErr
			}

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			require.Len(t, f.queue.enqueued, 1, "the rejected job must be retried")

			require.NotNil(t, condAtEnqueueTime)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, condAtEnqueueTime.Status,
				"TRUE must not be written before the queue has accepted the job")
			assert.Equal(t, constants.KueueQueueNotFound, condAtEnqueueTime.Reason)

			enqueued := findCondition(f.stored(t).Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
		})
	}

	byReason := []struct {
		name        string
		reason      string
		age         time.Duration
		wantRetried bool
	}{
		{
			name:   "a check that could not ask the cluster, a minute ago",
			reason: constants.KueueQueueCheckFailed,
			age:    time.Minute,
		},
		{
			name:        "a check that could not ask the cluster, six minutes ago",
			reason:      constants.KueueQueueCheckFailed,
			age:         6 * time.Minute,
			wantRetried: true,
		},
		{
			name:        "a LocalQueue that does not exist, a minute ago",
			reason:      constants.KueueQueueNotFound,
			age:         time.Minute,
			wantRetried: true,
		},
	}
	for _, tc := range byReason {
		t.Run("how long a rejection waits depends on its reason: "+tc.name, func(t *testing.T) {
			f := newReconcileFixture(t, rejectedSparkJob(tc.reason, tc.age))
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			stored := f.stored(t)
			enqueued := findCondition(stored.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			waits := f.logs.FilterMessage("scheduler queue rejected the spark job recently, not retrying yet").All()
			if !tc.wantRetried {
				assert.Empty(t, f.queue.enqueued, "the rejection is not old enough to be retried")
				assert.Equal(t, before, stored.ResourceVersion, "waiting out a rejection writes nothing")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued.Status)
				assert.Equal(t, tc.reason, enqueued.Reason)
				if assert.Len(t, waits, 1, "the wait is logged") {
					assert.Equal(t, zapcore.DebugLevel, waits[0].Level)
					assert.Equal(t, sparkStatusProbeInterval.String(), waits[0].ContextMap()["retry_after"])
				}
				return
			}
			assert.Len(t, f.queue.enqueued, 1, "the rejection is old enough to be retried")
			assert.Empty(t, waits)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
		})
	}

	t.Run("a retry the queue rejects again writes nothing and is not an error", func(t *testing.T) {
		f := newReconcileFixture(t, rejectedSparkJob(constants.KueueQueueNotFound, 30*time.Second))
		f.queue.enqueueFunc = func(context.Context, matypes.SchedulableJob) error {
			return fmt.Errorf("kueue validation: LocalQueue %q not found on cluster %q", "test-project", testClusterName)
		}
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err, "the pre-flight has recorded why; the requeue retries it")
		assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
		assert.Len(t, f.queue.enqueued, 1)
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion, "a failed retry must not write TRUE")
		enqueued := findCondition(stored.Status.StatusConditions, constants.EnqueuedCondition)
		require.NotNil(t, enqueued)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued.Status)
	})
}

// TestReconciler_Reconcile_WaitsForAssignment covers the gap between enqueueing
// and launching: without a reachable assigned cluster nothing is created.
func TestReconciler_Reconcile_WaitsForAssignment(t *testing.T) {
	t.Run("not scheduled yet", func(t *testing.T) {
		f := newReconcileFixture(t, testSparkJob(
			testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE),
		))

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, requeueAfter, res.RequeueAfter)
	})

	t.Run("assigned cluster is not registered", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, requeueAfter, res.RequeueAfter)
		assert.Empty(t, f.queue.enqueued, "a scheduled job must not be re-enqueued")
	})
}

// TestReconciler_Reconcile_Launch covers the projection of a scheduled SparkJob
// onto its assigned member cluster.
func TestReconciler_Reconcile_Launch(t *testing.T) {
	t.Run("creates the SparkApplication once and records Launched", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		assigned := testCluster()
		f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
		f.federated.EXPECT().
			CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), assigned).
			DoAndReturn(func(_ context.Context, jobObject, _ runtime.Object, _ *v2pb.Cluster) error {
				sparkJob, ok := jobObject.(*v2pb.SparkJob)
				require.True(t, ok, "the federated client must be handed the SparkJob itself")
				assert.Equal(t, sparkJobName, sparkJob.Name)
				assert.Equal(t, testNamespace, sparkJob.Namespace)
				// The finalizer's name is what an operator removes by hand after
				// rolling the controllermgr back, so its text is pinned here
				// rather than through the constant.
				assert.Equal(t, []string{"michelangelo/SparkApplicationCleanup"}, f.stored(t).Finalizers,
					"the cleanup finalizer must be stored before the SparkApplication is created")
				return nil
			}).
			Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res,
			"a launched job is probed for its outcome from now on")

		stored := f.stored(t)
		launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "Launched condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
		assert.Equal(t, reasonSparkApplicationCreated, launched.Reason)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers,
			"the cleanup finalizer stays on a launched job")
	})

	t.Run("an existing SparkApplication counts as launched", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
			Return(apiErrors.NewAlreadyExists(schema.GroupResource{Resource: "sparkapplications"}, sparkJobName))

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)

		launched := findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "Launched condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
	})

	t.Run("creation failure is terminal", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
			Return(errors.New("quota exceeded on member cluster"))

		res, err := f.reconcile()

		// The creation error is recorded, not returned: retrying it would only
		// fail the same way, so the job is driven to a terminal outcome instead.
		require.NoError(t, err)
		assert.Equal(t, requeueAfter, res.RequeueAfter)

		stored := f.stored(t)
		succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded, "Succeeded condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
		assert.Equal(t, reasonSparkApplicationCreationFailed, succeeded.Reason)
		assert.Contains(t, succeeded.Message, "quota exceeded on member cluster")
		launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "a job that was never created says so on the Launched condition")
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status,
			"a job that was never created must not claim to be launched")
		assert.Equal(t, reasonSparkApplicationCreationFailed, launched.Reason)

		// The next reconcile freezes the job and touches nothing else: no further
		// mock calls are declared, so any would fail the test.
		res, err = f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), res.RequeueAfter)
		assert.True(t, apiutils.IsImmutable(f.stored(t)), "a job that failed to launch should be frozen")
	})

	t.Run("a launched job that is still running is left to the federated watch", func(t *testing.T) {
		f := newReconcileFixture(t, launchedSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(probedSparkStatus(apipb.CONDITION_STATUS_UNKNOWN, "RUNNING", ""), nil)
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res,
			"a launched job is probed again, not left without a next tick")
		assert.Empty(t, f.queue.enqueued, "a launched job must not be enqueued")
		assert.Equal(t, before, f.stored(t).ResourceVersion, "the watch owns the status of a running job")
	})
}

// TestReconciler_Launch_CleanupFinalizerRechecksTheJob covers the fresh read
// the cleanup finalizer's write makes before the first CreateJob: a job frozen,
// deleted, terminated or finished since Reconcile read it gets neither the
// finalizer nor a SparkApplication, and is requeued for the arm the change
// calls for. launch is handed the copy from before the change, standing in for
// a cached read that lagged it. No CreateJob is declared, so a launch fails the
// test.
func TestReconciler_Launch_CleanupFinalizerRechecksTheJob(t *testing.T) {
	changes := []struct {
		name       string
		change     func(job *v2pb.SparkJob)
		wantReason string
	}{
		{
			name:       "the job was frozen",
			change:     func(job *v2pb.SparkJob) { apiutils.MarkImmutable(job) },
			wantReason: "job is immutable",
		},
		{
			// The API server's delete path marks the job and leaves the delete
			// to the ingester.
			name:       "the job is being deleted",
			change:     func(job *v2pb.SparkJob) { job.Annotations = map[string]string{api.DeletingAnnotation: "true"} },
			wantReason: "job is being deleted",
		},
		{
			name:       "a termination was requested",
			change:     func(job *v2pb.SparkJob) { job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED) },
			wantReason: "termination was requested",
		},
		{
			name: "an outcome was recorded",
			change: func(job *v2pb.SparkJob) {
				job.Status.StatusConditions = append(job.Status.StatusConditions,
					testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE))
			},
			wantReason: "outcome already recorded",
		},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			changed := scheduledSparkJob()
			tc.change(changed)
			f := newReconcileFixture(t, changed)
			before := f.stored(t).ResourceVersion

			res, err := f.r.launch(context.Background(), f.logger, scheduledSparkJob(), testCluster())

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			stored := f.stored(t)
			assert.Equal(t, before, stored.ResourceVersion, "nothing may be written")
			assert.Empty(t, stored.Finalizers)
			skipLogs := f.logs.FilterMessage("not launching the spark job").All()
			require.Len(t, skipLogs, 1)
			assert.Equal(t, tc.wantReason, skipLogs[0].ContextMap()["reason"])
		})
	}
}

// TestReconciler_Reconcile_LaunchFailures covers how a failed creation is
// handled by what the error says: a transient failure is recorded as a retry
// and returned for one; anything else is the job's outcome, recorded with a
// message fit for the job's authors. A creation that ran out of time also
// holds its cluster, which TestReconciler_Reconcile_ClusterHold covers; here
// the hold is lifted before the retry. TestReconciler_Reconcile_LaunchRetryRecord
// covers when the record of a transient failure is written.
func TestReconciler_Reconcile_LaunchFailures(t *testing.T) {
	transient := []struct {
		name        string
		createErr   error
		wantMessage string
		holds       bool
	}{
		{
			name: "the API server is unavailable",
			createErr: fmt.Errorf("create spark application err:%w",
				apiErrors.NewServiceUnavailable("etcdserver: leader changed")),
			wantMessage: "etcdserver: leader changed",
		},
		{
			name: "the request ran out of time",
			createErr: fmt.Errorf("create spark application err:%w",
				transportError(context.DeadlineExceeded)),
			wantMessage: `request to cluster "member-cluster-1" failed: context deadline exceeded`,
			holds:       true,
		},
	}
	for _, tc := range transient {
		t.Run("a transient failure is recorded as a retry, returned, and retried: "+tc.name, func(t *testing.T) {
			f := newReconcileFixture(t, scheduledSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
			gomock.InOrder(
				f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(tc.createErr),
				f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil),
			)

			res, err := f.reconcile()

			require.ErrorIs(t, err, tc.createErr, "a transient failure is returned for the controller's backoff")
			assert.Equal(t, ctrl.Result{}, res)
			stored := f.stored(t)
			launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched, "a transient failure is recorded on the Launched condition")
			assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, launched.Status)
			assert.Equal(t, reasonSparkApplicationCreationRetrying, launched.Reason)
			assert.Equal(t, tc.wantMessage, launched.Message)
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.SucceededCondition),
				"a transient failure is not the job's outcome")
			assert.False(t, apiutils.IsImmutable(stored), "a transient failure must not freeze the job")
			assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers,
				"the cleanup finalizer stays on a job whose creation is retried")
			_, held := f.r.clusterHeldUntil(testClusterName, time.Now())
			assert.Equal(t, tc.holds, held, "only a creation that ran out of time holds its cluster")

			// The retry creates the application and records the launch, once
			// any hold the failure started has ended.
			delete(f.r.clusterHolds, testClusterName)
			res, err = f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
			launched = findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched, "Launched condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
			assert.Equal(t, reasonSparkApplicationCreated, launched.Reason)
			assert.Empty(t, launched.Message, "the launch replaces the record of the failure")
		})
	}

	forbidden := apiErrors.NewForbidden(sparkApplicationsResource, sparkJobName,
		errors.New(`admission webhook "validate.sparkoperator.k8s.io" denied the request: spec.driver.cores must be positive`))
	conflict := fmt.Errorf("%w: default/%s on cluster %q belongs to job UID %q",
		jobsclient.ErrSparkApplicationConflict, sparkJobName, testClusterName, "another-job-uid")
	terminal := []struct {
		name        string
		createErr   error
		wantMessage string
	}{
		{
			name:        "a rejection is recorded with the API server's message",
			createErr:   fmt.Errorf("create spark application err:%w", forbidden),
			wantMessage: forbidden.Status().Message,
		},
		{
			name: "a request that got no answer is recorded without the member cluster's endpoint",
			createErr: fmt.Errorf("create spark application err:%w",
				transportError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"))),
			wantMessage: `request to cluster "member-cluster-1" failed: tls: failed to verify certificate: x509: certificate signed by unknown authority`,
		},
		{
			name:        "another job's SparkApplication under the same name is terminal",
			createErr:   conflict,
			wantMessage: conflict.Error(),
		},
	}
	for _, tc := range terminal {
		t.Run(tc.name, func(t *testing.T) {
			f := newReconcileFixture(t, scheduledSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(tc.createErr)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, requeueAfter, res.RequeueAfter)
			stored := f.stored(t)
			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded, "Succeeded condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, reasonSparkApplicationCreationFailed, succeeded.Reason)
			assert.Equal(t, tc.wantMessage, succeeded.Message)
			assert.NotContains(t, succeeded.Message, "10.20.30.40")
			launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched, "a creation that failed for good is recorded on the Launched condition too")
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
			assert.Equal(t, reasonSparkApplicationCreationFailed, launched.Reason)
			assert.Equal(t, tc.wantMessage, launched.Message)

			// The next reconcile freezes the job, with no further remote call.
			_, err = f.reconcile()

			require.NoError(t, err)
			assert.True(t, apiutils.IsImmutable(f.stored(t)), "a job that failed to launch should be frozen")
		})
	}
}

// connectionResetFrom is the error a creation fails with when the connection
// it went out on, from the given local port, is reset by the member cluster.
func connectionResetFrom(port int) error {
	return fmt.Errorf("create spark application err:%w", transportError(&net.OpError{
		Op:     "read",
		Net:    "tcp",
		Source: &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: port},
		Addr:   &net.TCPAddr{IP: net.ParseIP("10.20.30.40"), Port: 6443},
		Err:    os.NewSyscallError("read", syscall.ECONNRESET),
	}))
}

// TestReconciler_Reconcile_LaunchRetryRecord covers the record of a transient
// creation failure: Launched=UNKNOWN with reasonSparkApplicationCreationRetrying
// and the failure's sanitized message. A failure like the one before writes
// nothing, and one that reads differently replaces the record only once the
// record is sparkStatusProbeInterval old, so no watch event of the record's
// own keeps cutting the controller's backoff short. Launched=TRUE and a
// terminal failure replace it. It is never written onto a job that must not be
// written to, nor over a launch that a stale copy of the job did not show, and
// a failed write of it leaves the creation's error to be returned.
func TestReconciler_Reconcile_LaunchRetryRecord(t *testing.T) {
	const resetMessage = `request to cluster "member-cluster-1" failed: read tcp: read: connection reset by peer`
	requireRecord := func(t *testing.T, job *v2pb.SparkJob, wantMessage string) {
		t.Helper()
		launched := findCondition(job.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "a transient failure is recorded on the Launched condition")
		assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, launched.Status)
		// The reason is what the job's authors and their tools see, so its
		// text is pinned here rather than through the constant.
		assert.Equal(t, "SparkApplicationCreationRetrying", launched.Reason)
		assert.Equal(t, wantMessage, launched.Message)
	}
	// heldScheduledSparkJob is a scheduled job that already carries the
	// cleanup finalizer, so that launch goes straight to the creation.
	heldScheduledSparkJob := func(conditions ...*apipb.Condition) *v2pb.SparkJob {
		job := scheduledSparkJob(conditions...)
		job.Finalizers = []string{sparkApplicationCleanupFinalizer}
		return job
	}

	t.Run("a repeat is never written, a failure that says something new only once the record is old, and the launch replaces the record", func(t *testing.T) {
		unavailable := fmt.Errorf("create spark application err:%w", apiErrors.NewServiceUnavailable("etcdserver: leader changed"))
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(6)
		gomock.InOrder(
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(connectionResetFrom(54321)),
			// The same failure, on a connection from another local port.
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(connectionResetFrom(54322)),
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(unavailable).Times(3),
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil),
		)

		_, err := f.reconcile()

		require.Error(t, err)
		first := f.stored(t)
		requireRecord(t, first, resetMessage)

		_, err = f.reconcile()

		require.Error(t, err)
		assert.Equal(t, first.ResourceVersion, f.stored(t).ResourceVersion, "a repeated failure must write nothing")

		_, err = f.reconcile()

		require.ErrorIs(t, err, unavailable)
		assert.Equal(t, first.ResourceVersion, f.stored(t).ResourceVersion,
			"a failure that says something new must not replace a record written moments ago")

		ageLaunchedCondition(t, f, sparkStatusProbeInterval-time.Minute)
		aged := f.stored(t)
		_, err = f.reconcile()

		require.ErrorIs(t, err, unavailable)
		assert.Equal(t, aged.ResourceVersion, f.stored(t).ResourceVersion,
			"a record younger than sparkStatusProbeInterval is kept")
		requireRecord(t, f.stored(t), resetMessage)

		ageLaunchedCondition(t, f, sparkStatusProbeInterval)
		aged = f.stored(t)
		_, err = f.reconcile()

		require.ErrorIs(t, err, unavailable)
		second := f.stored(t)
		assert.NotEqual(t, aged.ResourceVersion, second.ResourceVersion,
			"a failure that says something new replaces a record sparkStatusProbeInterval old")
		requireRecord(t, second, "etcdserver: leader changed")
		launched := findCondition(second.Status.StatusConditions, constants.LaunchedCondition)
		assert.WithinDuration(t, time.Now(), time.Unix(launched.LastUpdatedTimestamp, 0), time.Minute,
			"the new record is stamped when it was written")

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		launched = findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
		assert.Equal(t, reasonSparkApplicationCreated, launched.Reason)
		assert.Empty(t, launched.Message, "the launch replaces the record of the failure")
	})

	t.Run("a creation that then fails for good replaces the record with Launched=FALSE", func(t *testing.T) {
		forbidden := apiErrors.NewForbidden(sparkApplicationsResource, sparkJobName,
			errors.New(`admission webhook "validate.sparkoperator.k8s.io" denied the request: spec.driver.cores must be positive`))
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		gomock.InOrder(
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(connectionResetFrom(54321)),
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
				Return(fmt.Errorf("create spark application err:%w", forbidden)),
		)
		_, err := f.reconcile()
		require.Error(t, err)
		requireRecord(t, f.stored(t), resetMessage)

		_, err = f.reconcile()

		require.NoError(t, err)
		stored := f.stored(t)
		launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
		assert.Equal(t, reasonSparkApplicationCreationFailed, launched.Reason)
		assert.Equal(t, forbidden.Status().Message, launched.Message)
		succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
	})

	// The next two cases run launch directly on a copy of the job that
	// Reconcile read before the stored one changed.
	t.Run("nothing is recorded on a job a termination was requested for since it was read", func(t *testing.T) {
		stored := heldScheduledSparkJob()
		stored.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixture(t, stored)
		createErr := connectionResetFrom(54321)
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(createErr)
		before := f.stored(t).ResourceVersion

		_, err := f.r.launch(log.IntoContext(context.Background(), f.logger), f.logger, heldScheduledSparkJob(), testCluster())

		require.ErrorIs(t, err, createErr)
		after := f.stored(t)
		assert.Equal(t, before, after.ResourceVersion, "nothing may be written onto a job being terminated")
		assert.Nil(t, findCondition(after.Status.StatusConditions, constants.LaunchedCondition))
		skips := f.logs.FilterMessage("not recording the retried creation").All()
		require.Len(t, skips, 1)
		assert.Equal(t, "termination was requested", skips[0].ContextMap()["reason"])
	})

	t.Run("a launch that the job's copy did not show is not taken back", func(t *testing.T) {
		f := newReconcileFixture(t, heldScheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE)))
		createErr := connectionResetFrom(54321)
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(createErr)
		before := f.stored(t).ResourceVersion

		_, err := f.r.launch(log.IntoContext(context.Background(), f.logger), f.logger, heldScheduledSparkJob(), testCluster())

		require.ErrorIs(t, err, createErr)
		after := f.stored(t)
		assert.Equal(t, before, after.ResourceVersion)
		launched := findCondition(after.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status, "Launched=TRUE is never taken back")
	})

	t.Run("a record that cannot be written is logged, and the creation's error is returned", func(t *testing.T) {
		injected := errors.New("etcdserver: request timed out")
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
				return injected
			},
		}, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		createErr := connectionResetFrom(54321)
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(createErr)

		res, err := f.reconcile()

		require.ErrorIs(t, err, createErr)
		assert.NotErrorIs(t, err, injected, "the creation's error is returned, not the record's")
		assert.Equal(t, ctrl.Result{}, res)
		writeErrors := f.logs.FilterMessage("failed to record the retried creation of the spark application").All()
		require.Len(t, writeErrors, 1)
		assert.Equal(t, zapcore.ErrorLevel, writeErrors[0].Level)
		assert.Equal(t, injected.Error(), writeErrors[0].ContextMap()["error"])
	})
}

// TestReconciler_Reconcile_StatusWriteFailures injects failures into the
// controller's own status writes.
func TestReconciler_Reconcile_StatusWriteFailures(t *testing.T) {
	t.Run("a conflicting write is redone on a fresh copy, keeping what the watch recorded meanwhile", func(t *testing.T) {
		attempts := 0
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				attempts++
				if attempts == 1 {
					// Stand in for the watch: it records the application as
					// running between the controller's read and its write, which
					// leaves the controller's copy stale.
					var current v2pb.SparkJob
					require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), &current))
					running := testCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE)
					running.Reason = "RUNNING"
					current.Status.StatusConditions = append(current.Status.StatusConditions, running)
					require.NoError(t, c.SubResource(subResource).Update(ctx, &current))
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		assert.Equal(t, 2, attempts, "the write must be redone once, after the conflict")
		stored := f.stored(t)
		launched := findCondition(stored.Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "Launched condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
		running := findCondition(stored.Status.StatusConditions, constants.SparkAppRunningCondition)
		require.NotNil(t, running, "the watch's condition must survive the controller's redone write")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, running.Status)
	})

	t.Run("a failed Launched write is returned, and the next reconcile records the launch", func(t *testing.T) {
		injected := errors.New("etcdserver: request timed out")
		failures := 1
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if failures > 0 {
					failures--
					return injected
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		gomock.InOrder(
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil),
			// The application the first attempt created is still there.
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
				Return(apiErrors.NewAlreadyExists(sparkApplicationsResource, sparkJobName)),
		)

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.LaunchedCondition))
		assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.SucceededCondition),
			"a lost Launched write is not a failed job")

		res, err = f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		launched := findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "Launched condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
	})

	t.Run("a write that keeps conflicting requeues the job without an error, and the next reconcile records the launch", func(t *testing.T) {
		attempts := 0
		conflicting := true
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if conflicting {
					// Another writer changes the job before every attempt reads it.
					attempts++
					return conflictError()
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		gomock.InOrder(
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil),
			// The application the first attempt created is still there.
			f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
				Return(apiErrors.NewAlreadyExists(sparkApplicationsResource, sparkJobName)),
		)

		res, err := f.reconcile()

		require.NoError(t, err, "a conflict is routine, not a reconcile error")
		assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
		assert.Equal(t, retry.DefaultBackoff.Steps, attempts, "the write is retried for as long as retry.DefaultBackoff allows")
		assert.Nil(t, findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition))
		assert.Empty(t, f.logs.FilterLevelExact(zapcore.ErrorLevel).All(), "a conflict is not logged as an error")
		for _, msg := range []string{
			"failed to update launched condition",
			"a write to the spark job conflicted with a concurrent write, requeueing",
		} {
			entries := f.logs.FilterMessage(msg).All()
			require.Len(t, entries, 1, msg)
			assert.Equal(t, zapcore.DebugLevel, entries[0].Level, msg)
			assert.Equal(t, conflictError().Error(), entries[0].ContextMap()["error"], msg)
		}

		conflicting = false
		res, err = f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		launched := findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
		require.NotNil(t, launched, "Launched condition should be set")
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
	})

	t.Run("a termination whose write keeps conflicting logs no error and is requeued", func(t *testing.T) {
		job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
				return conflictError()
			},
		}, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
		assert.Empty(t, f.logs.FilterLevelExact(zapcore.ErrorLevel).All(), "a conflict is not logged as an error")
		entries := f.logs.FilterMessage("failed to update SparkJob status").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zapcore.DebugLevel, entries[0].Level)
		assert.Equal(t, sparkJobName, entries[0].ContextMap()["name"], "the write's own fields are kept")
	})
}

// TestMarkSparkApplicationCreationRetrying pins when the record of a transient
// creation failure is written: always over no record, never over a launch or a
// record of the same failure, and over the record of another failure only once
// that record is sparkStatusProbeInterval old.
func TestMarkSparkApplicationCreationRetrying(t *testing.T) {
	const message = `request to cluster "member-cluster-1" failed: read tcp: read: connection reset by peer`
	// A whole second, so that a record stamped exactly the interval before is
	// exactly the interval old.
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	launchedCondition := func(status apipb.ConditionStatus, reason, message string, age time.Duration) *apipb.Condition {
		launched := testCondition(constants.LaunchedCondition, status)
		launched.Reason = reason
		launched.Message = message
		launched.LastUpdatedTimestamp = now.Add(-age).Unix()
		return launched
	}
	retrying := func(message string, age time.Duration) *apipb.Condition {
		return launchedCondition(apipb.CONDITION_STATUS_UNKNOWN, reasonSparkApplicationCreationRetrying, message, age)
	}

	for _, tc := range []struct {
		name     string
		launched *apipb.Condition
		want     bool
	}{
		{name: "no Launched condition yet", want: true},
		{name: "a record of the same failure, however old", launched: retrying(message, time.Hour), want: false},
		{name: "a record of another failure just short of the interval old", launched: retrying("etcdserver: leader changed", sparkStatusProbeInterval-time.Second), want: false},
		{name: "a record of another failure the interval old", launched: retrying("etcdserver: leader changed", sparkStatusProbeInterval), want: true},
		{name: "an UNKNOWN Launched that is no record, however recent", launched: launchedCondition(apipb.CONDITION_STATUS_UNKNOWN, "", "", 0), want: true},
		{name: "a launch", launched: launchedCondition(apipb.CONDITION_STATUS_TRUE, reasonSparkApplicationCreated, "", time.Hour), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := scheduledSparkJob()
			if tc.launched != nil {
				job.Status.StatusConditions = append(job.Status.StatusConditions, tc.launched)
			}
			before := job.DeepCopy()

			got := markSparkApplicationCreationRetrying(job, message, now)

			assert.Equal(t, tc.want, got)
			if !tc.want {
				assert.Equal(t, before, job, "a mutation that declines changes nothing")
				return
			}
			launched := findCondition(job.Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched)
			assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, launched.Status)
			assert.Equal(t, reasonSparkApplicationCreationRetrying, launched.Reason)
			assert.Equal(t, message, launched.Message)
		})
	}
}

// TestReconciler_Reconcile_Guards covers the two arms that run before anything
// else: a frozen job, and a job whose outcome is already written down.
func TestReconciler_Reconcile_Guards(t *testing.T) {
	t.Run("immutable job is untouched", func(t *testing.T) {
		job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		apiutils.MarkImmutable(job)
		f := newReconcileFixture(t, job)
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		assert.Empty(t, f.queue.enqueued)
		assert.Equal(t, before, f.stored(t).ResourceVersion)
	})

	outcomes := map[string]apipb.ConditionStatus{
		"succeeded": apipb.CONDITION_STATUS_TRUE,
		"failed":    apipb.CONDITION_STATUS_FALSE,
	}
	for name, status := range outcomes {
		t.Run("a recorded outcome freezes the job: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, scheduledSparkJob(
				testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
				testCondition(constants.SucceededCondition, status),
			))

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			assert.Empty(t, f.queue.enqueued)
			assert.True(t, apiutils.IsImmutable(f.stored(t)))
		})
	}

	t.Run("a termination request cannot overwrite a recorded outcome", func(t *testing.T) {
		job := scheduledSparkJob(
			testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
			testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE),
		)
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixture(t, job)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)

		stored := f.stored(t)
		succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeeded.Status, "the recorded outcome must survive")
		assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition),
			"a job that already finished must not also be marked killed")
		assert.True(t, apiutils.IsImmutable(stored))
	})
}

// TestReconciler_Reconcile_Termination verifies that a SparkJob with
// Spec.Termination set drives the reconciler to delete the SparkApplication it
// projected onto the assigned member cluster and to transition the SparkJob to
// a terminal, immutable state.
func TestReconciler_Reconcile_Termination(t *testing.T) {
	tests := []struct {
		name            string
		deleteJobErr    error
		terminationType v2pb.TerminationType
		wantSucceeded   apipb.ConditionStatus
	}{
		{
			name:            "failed termination deletes SparkApplication, marks killed and not-succeeded",
			deleteJobErr:    nil,
			terminationType: v2pb.TERMINATION_TYPE_FAILED,
			wantSucceeded:   apipb.CONDITION_STATUS_FALSE,
		},
		{
			name:            "succeeded termination marks killed and succeeded",
			deleteJobErr:    nil,
			terminationType: v2pb.TERMINATION_TYPE_SUCCEEDED,
			wantSucceeded:   apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:            "termination tolerates an already-deleted SparkApplication",
			deleteJobErr:    apiErrors.NewNotFound(schema.GroupResource{Resource: "sparkapplications"}, sparkJobName),
			terminationType: v2pb.TERMINATION_TYPE_FAILED,
			wantSucceeded:   apipb.CONDITION_STATUS_FALSE,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
			job.Spec.Termination = testTermination(tc.terminationType)

			f := newReconcileFixture(t, job)
			assigned := testCluster()
			f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), assigned).
				DoAndReturn(func(_ context.Context, jobObject runtime.Object, _ *v2pb.Cluster) error {
					sparkJob, ok := jobObject.(*v2pb.SparkJob)
					require.True(t, ok, "the federated client must be handed the SparkJob itself")
					assert.Equal(t, sparkJobName, sparkJob.Name)
					return tc.deleteJobErr
				}).
				Times(1)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, time.Duration(0), res.RequeueAfter, "terminal SparkJob should not be requeued")

			stored := f.stored(t)

			killed := findCondition(stored.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, constants.SparkAppKilled, killed.Reason)
			assert.Equal(t, "workflow cancelled", killed.Message)

			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded, "Succeeded condition should be set")
			assert.Equal(t, tc.wantSucceeded, succeeded.Status)
			assert.Equal(t, "workflow cancelled", succeeded.Reason)

			assert.True(t, apiutils.IsImmutable(stored), "terminated SparkJob should be marked immutable")
		})
	}
}

// TestReconciler_Reconcile_TerminationWithoutLaunched covers a termination that
// does not trust the Launched condition: once the job is scheduled to a cluster
// the cache resolves, terminating it deletes the SparkApplication whether or
// not Launched is visible, since the flag can lag the application. A NotFound
// means there was nothing to delete.
func TestReconciler_Reconcile_TerminationWithoutLaunched(t *testing.T) {
	deleteResults := map[string]error{
		"the SparkApplication is deleted": nil,
		"there was no SparkApplication":   apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName),
	}
	for name, deleteErr := range deleteResults {
		t.Run(name, func(t *testing.T) {
			job := scheduledSparkJob()
			job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
			f := newReconcileFixture(t, job)
			assigned := testCluster()
			f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), assigned).Return(deleteErr).Times(1)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)

			stored := f.stored(t)
			killed := findCondition(stored.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, "workflow cancelled", killed.Message)
			assert.True(t, apiutils.IsImmutable(stored), "terminated SparkJob should be marked immutable")
		})
	}
}

// TestReconciler_Reconcile_TerminationClusterCacheMiss covers a termination
// whose assigned cluster the cluster cache cannot resolve. A miss is not taken
// for a deregistered cluster: the cluster's Cluster object is read, and while
// it exists the SparkApplication is deleted through it and the kill is
// recorded as usual. Only a cluster whose Cluster object is gone is taken to be
// deregistered, and the kill is recorded without a delete. A read that fails
// records nothing and is returned. Launched makes no difference.
func TestReconciler_Reconcile_TerminationClusterCacheMiss(t *testing.T) {
	jobs := map[string]func() *v2pb.SparkJob{
		"launched": func() *v2pb.SparkJob {
			return scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		},
		"not launched": func() *v2pb.SparkJob { return scheduledSparkJob() },
	}
	for name, newJob := range jobs {
		t.Run("a cluster whose Cluster object exists has the SparkApplication deleted through it: "+name, func(t *testing.T) {
			job := newJob()
			job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
			f := newReconcileFixture(t, job, registeredCluster())
			f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), isRegisteredCluster{}).Return(nil).Times(1)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			stored := f.stored(t)
			killed := findCondition(stored.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, "workflow cancelled", killed.Message)
			assert.True(t, apiutils.IsImmutable(stored), "terminated SparkJob should be marked immutable")
		})

		t.Run("a cluster with no Cluster object is recorded as killed without a delete: "+name, func(t *testing.T) {
			job := newJob()
			job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
			f := newReconcileFixture(t, job)
			f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
			// No DeleteJob is declared: there is no cluster to call.

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			stored := f.stored(t)
			killed := findCondition(stored.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, unregisteredClusterMessage, killed.Message)
			assert.True(t, apiutils.IsImmutable(stored), "terminated SparkJob should be marked immutable")
		})
	}

	t.Run("a Cluster object that cannot be read records nothing and is returned for a retry", func(t *testing.T) {
		job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		injected := errors.New("cluster read failed")
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			Get: failClusterReads(injected),
		}, job, registeredCluster())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		// No DeleteJob is declared: nothing is called while the cluster is unknown.
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion, "nothing may be recorded while the cluster is unknown")
		assert.False(t, apiutils.IsImmutable(stored))
	})
}

// TestReconciler_Reconcile_TerminationWriteGuardKeepsAnOutcome checks the
// defensive guard in the termination's status write: the kill goes onto the
// freshest copy of the job and is not written over an outcome that copy
// already records. No writer in this package records an outcome once a
// termination is requested, since the watch and the probe both skip such a
// job, so the test plants one directly while the delete is in flight. The
// planted outcome is kept, no Killed condition is added, and the job is frozen
// either way.
func TestReconciler_Reconcile_TerminationWriteGuardKeepsAnOutcome(t *testing.T) {
	job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
	job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
	f := newReconcileFixture(t, job)
	f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
	f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, runtime.Object, *v2pb.Cluster) error {
			recordOutcome(t, f, apipb.CONDITION_STATUS_TRUE, "COMPLETED")
			return nil
		})

	res, err := f.reconcile()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)

	stored := f.stored(t)
	succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
	require.NotNil(t, succeeded)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeeded.Status, "the recorded outcome must survive")
	assert.Equal(t, "COMPLETED", succeeded.Reason)
	assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition),
		"a job that finished must not also be marked killed")
	assert.True(t, apiutils.IsImmutable(stored), "the job must be frozen either way")
}

// TestReconciler_HandleTermination_FreezesAKilledJob covers a job already
// recorded as killed but not yet frozen: there is nothing left to delete or
// record, but the freeze is a write of its own that may not have happened, so
// handleTermination still makes it. Reconcile's outcome arm catches a killed
// job first, which is why the function is called directly.
func TestReconciler_HandleTermination_FreezesAKilledJob(t *testing.T) {
	job := scheduledSparkJob(
		testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
		testCondition(constants.KilledCondition, apipb.CONDITION_STATUS_TRUE),
		testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE),
	)
	job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
	f := newReconcileFixture(t, job)
	// No cache or federated-client call is declared: there is nothing to delete.

	res, err := f.r.handleTermination(context.Background(), logr.Discard(), f.stored(t))

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
	assert.True(t, apiutils.IsImmutable(f.stored(t)), "a killed job must still be frozen")
}

// TestReconciler_Reconcile_TerminationWithoutWorkload covers killing a SparkJob
// whose SparkApplication the controller cannot delete: the job never reached a
// member cluster, or the one it was assigned is no longer registered with the
// control plane. The controller records the kill and says why no delete
// happened.
func TestReconciler_Reconcile_TerminationWithoutWorkload(t *testing.T) {
	tests := []struct {
		name        string
		job         func() *v2pb.SparkJob
		setup       func(f *reconcileFixture)
		wantMessage string
	}{
		{
			name: "never scheduled",
			job: func() *v2pb.SparkJob {
				return testSparkJob(testCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE))
			},
			setup:       func(*reconcileFixture) {},
			wantMessage: constants.KilledMessageJobNotLaunched,
		},
		{
			name: "assigned cluster is no longer registered",
			job:  func() *v2pb.SparkJob { return scheduledSparkJob() },
			setup: func(f *reconcileFixture) {
				f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
			},
			wantMessage: unregisteredClusterMessage,
		},
		{
			name: "launched on a cluster that is no longer registered",
			job: func() *v2pb.SparkJob {
				return scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
			},
			setup: func(f *reconcileFixture) {
				f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
			},
			wantMessage: unregisteredClusterMessage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := tc.job()
			job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)

			f := newReconcileFixture(t, job)
			tc.setup(f)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, time.Duration(0), res.RequeueAfter)

			stored := f.stored(t)
			killed := findCondition(stored.Status.StatusConditions, constants.KilledCondition)
			require.NotNil(t, killed, "Killed condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, killed.Status)
			assert.Equal(t, tc.wantMessage, killed.Message)

			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded, "Succeeded condition should be set")
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)

			assert.True(t, apiutils.IsImmutable(stored), "terminated SparkJob should be marked immutable")
		})
	}

	t.Run("a job killed before it was ever enqueued does not reach the scheduler", func(t *testing.T) {
		job := testSparkJob()
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixture(t, job)

		_, err := f.reconcile()

		require.NoError(t, err)
		assert.Empty(t, f.queue.enqueued, "a terminating job must never be enqueued")
	})
}

// TestReconciler_Reconcile_TerminationDeleteFails covers a delete that fails for
// a reason other than the workload being gone: the kill is retried rather than
// recorded, so nothing terminal is written. A 404 counts as gone only when it
// carries a Kubernetes Status (isSparkApplicationAbsent).
func TestReconciler_Reconcile_TerminationDeleteFails(t *testing.T) {
	failures := map[string]error{
		"the member cluster is unreachable": errors.New("member cluster unreachable"),
		// The federated client returns the 404 of its read before the delete
		// as it is.
		"a 404 whose body is not a Kubernetes status": unstructuredNotFound(),
		// Nothing between the controller and a member cluster speaks gRPC, so
		// a NotFound in a gRPC status is not the member cluster's answer.
		"a not-found reported over gRPC": grpcstatus.Error(codes.NotFound, "resource not found"),
	}
	for name, deleteErr := range failures {
		t.Run(name, func(t *testing.T) {
			job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
			job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)

			f := newReconcileFixture(t, job)
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(deleteErr)

			res, err := f.reconcile()

			require.ErrorIs(t, err, deleteErr)
			// controller-runtime discards a RequeueAfter returned beside an
			// error, so none is returned; the retry runs on the controller's
			// backoff.
			assert.Equal(t, ctrl.Result{}, res)

			stored := f.stored(t)
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition),
				"a job whose workload survived must not be recorded as killed")
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.SucceededCondition),
				"a job whose workload survived has no outcome yet")
			assert.False(t, apiutils.IsImmutable(stored), "the kill has to stay retriable")
		})
	}
}

// newDeletingFixture stores job and marks it as being deleted, by marker: a
// deletion timestamp, or the deleting annotation the API server's delete path
// leaves for the ingester.
func newDeletingFixture(t *testing.T, marker string, job *v2pb.SparkJob) *reconcileFixture {
	t.Helper()
	switch marker {
	case "deletion timestamp":
		job.Finalizers = []string{controllerTestFinalizer}
		f := newReconcileFixture(t, job)
		require.NoError(t, f.r.Delete(context.Background(), f.stored(t)))
		require.NotNil(t, f.stored(t).GetDeletionTimestamp())
		return f
	case "deleting annotation":
		job.Annotations = map[string]string{api.DeletingAnnotation: "true"}
		return newReconcileFixture(t, job)
	}
	require.FailNow(t, "unknown deletion marker", marker)
	return nil
}

// TestReconciler_Reconcile_Deleting covers a SparkJob on its way out: it
// never reaches the scheduler or a member cluster's create path again, its
// SparkApplication is deleted when the job was scheduled to a cluster the
// control plane can reach, and nothing is written to its status.
func TestReconciler_Reconcile_Deleting(t *testing.T) {
	for _, marker := range []string{"deletion timestamp", "deleting annotation"} {
		t.Run("a scheduled job's SparkApplication is deleted and the job never launches: "+marker, func(t *testing.T) {
			f := newDeletingFixture(t, marker, scheduledSparkJob())
			assigned := testCluster()
			f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), assigned).Return(nil).Times(1)
			// No CreateJob is declared: a launch would fail the test.
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			assert.Empty(t, f.queue.enqueued, "a job being deleted must not be enqueued")
			stored := f.stored(t)
			assert.Equal(t, before, stored.ResourceVersion, "nothing may be written to a job being deleted")
			assert.NotContains(t, stored.Finalizers, sparkApplicationCleanupFinalizer,
				"a job deleted before its launch is never given the cleanup finalizer")
		})

		t.Run("an unscheduled job touches no member cluster and is not enqueued: "+marker, func(t *testing.T) {
			f := newDeletingFixture(t, marker, testSparkJob())
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			assert.Empty(t, f.queue.enqueued, "a job being deleted must not be enqueued")
			assert.Equal(t, before, f.stored(t).ResourceVersion, "nothing may be written to a job being deleted")
		})
	}

	t.Run("a launched job's SparkApplication that is already gone is no error", func(t *testing.T) {
		f := newDeletingFixture(t, "deletion timestamp",
			scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE)))
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName)).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
	})

	failedDeletes := map[string]error{
		"the member cluster is unreachable": errors.New("member cluster unreachable"),
		// A 404 without a Kubernetes Status is not proof that the application
		// is gone: a proxy may have sent it while the application runs on.
		"a 404 whose body is not a Kubernetes status": unstructuredNotFound(),
	}
	for name, injected := range failedDeletes {
		t.Run("a failed delete is returned for a retry, with nothing written: "+name, func(t *testing.T) {
			f := newDeletingFixture(t, "deletion timestamp",
				scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE)))
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(injected)
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.ErrorIs(t, err, injected)
			assert.Equal(t, ctrl.Result{}, res)
			assert.Equal(t, before, f.stored(t).ResourceVersion, "nothing may be written to a job being deleted")
		})
	}

	t.Run("a termination request on a job being deleted records no kill", func(t *testing.T) {
		job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newDeletingFixture(t, "deletion timestamp", job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion, "nothing may be written to a job being deleted")
		assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition))
	})

	t.Run("an assigned cluster missing from the cache has the SparkApplication deleted through its Cluster object", func(t *testing.T) {
		f := newDeletingFixture(t, "deletion timestamp", scheduledSparkJob())
		require.NoError(t, f.r.Create(context.Background(), registeredCluster()))
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), isRegisteredCluster{}).Return(nil).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
	})

	t.Run("an assigned cluster missing from the cache with no Cluster object is given up on", func(t *testing.T) {
		f := newDeletingFixture(t, "deletion timestamp", scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		// No DeleteJob is declared: there is no cluster to call.

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
	})
}

// heldLaunchedSparkJob builds a SparkJob like launchedSparkJob that carries the
// cleanup finalizer, as launch leaves every job it has launched.
func heldLaunchedSparkJob() *v2pb.SparkJob {
	job := launchedSparkJob()
	job.Finalizers = []string{sparkApplicationCleanupFinalizer}
	return job
}

// deletedSparkJob marks job as deleted the given time ago and held by the
// cleanup finalizer, as the API server leaves a deleted job until its last
// finalizer is removed. The fake client stores an object with a deletion
// timestamp only alongside a finalizer.
func deletedSparkJob(job *v2pb.SparkJob, ago time.Duration) *v2pb.SparkJob {
	deletedAt := metav1.NewTime(time.Now().Add(-ago))
	job.DeletionTimestamp = &deletedAt
	if !slices.Contains(job.Finalizers, sparkApplicationCleanupFinalizer) {
		job.Finalizers = append(job.Finalizers, sparkApplicationCleanupFinalizer)
	}
	return job
}

// requireGone asserts that the SparkJob named sparkJobName is no longer stored.
func (f *reconcileFixture) requireGone(t *testing.T) {
	t.Helper()
	err := f.r.Get(context.Background(), sparkJobKeyNamed(sparkJobName), &v2pb.SparkJob{})
	require.True(t, apiErrors.IsNotFound(err), "the SparkJob must be gone, got %v", err)
}

// TestReconciler_Reconcile_DeletionCleanupFinalizer covers the cleanup
// finalizer holding a deleted job until its SparkApplication is torn down. It
// is removed once the delete succeeds or finds nothing to delete, once the
// assigned cluster is missing from both the cache and the API server, and, as
// a last resort, once the deletion is more than an hour old and the delete
// keeps failing. The fake API
// server deletes a job whose deletion is pending as soon as its last finalizer
// is removed, as the real one does.
func TestReconciler_Reconcile_DeletionCleanupFinalizer(t *testing.T) {
	const giveUpMessage = "gave up deleting the SparkApplication of a deleted SparkJob, which may still be running: delete it by hand"

	// The federated client also returns nil, deleting nothing, for an
	// application under the job's name that is not the job's own.
	tornDown := map[string]error{
		"the SparkApplication is deleted":      nil,
		"the SparkApplication is already gone": apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName),
	}
	for name, deleteErr := range tornDown {
		t.Run("deleting a launched job deletes its SparkApplication, then the job goes: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, heldLaunchedSparkJob())
			require.NoError(t, f.r.Delete(context.Background(), f.stored(t)))
			require.NotNil(t, f.stored(t).GetDeletionTimestamp(), "the finalizer must keep the deleted job for its teardown")
			assigned := testCluster()
			f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), assigned).Return(deleteErr).Times(1)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			f.requireGone(t)
			assert.Empty(t, f.logs.FilterMessage(giveUpMessage).All())
		})
	}

	ages := map[string]time.Duration{
		"deleted just now":       0,
		"deleted 59 minutes ago": 59 * time.Minute,
	}
	for name, age := range ages {
		t.Run("a failed delete keeps the finalizer and is returned for a retry: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), age))
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			injected := errors.New("member cluster unreachable")
			f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(injected)
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.ErrorIs(t, err, injected)
			assert.Equal(t, ctrl.Result{}, res)
			stored := f.stored(t)
			assert.Equal(t, before, stored.ResourceVersion, "nothing may be written while the delete fails")
			assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers)
			assert.Empty(t, f.logs.FilterMessage(giveUpMessage).All())
		})
	}

	t.Run("a delete answered with a proxy's 409 is returned for a retry, not requeued as a write conflict", func(t *testing.T) {
		// client-go reads a 409 whose body is not a Kubernetes Status as a
		// Conflict. That one is a failed call to the member cluster, not a
		// write to the job that conflicted, so it goes back on the controller's
		// backoff like any other failed delete.
		f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), time.Minute))
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		proxyConflict := apiErrors.NewGenericServerResponse(409, "DELETE", sparkApplicationsResource, sparkJobName,
			"<html>409 Conflict</html>", 0, true)
		require.True(t, apiErrors.IsConflict(proxyConflict), "the case needs a 409 that reads as a Conflict")
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(proxyConflict)

		res, err := f.reconcile()

		require.ErrorIs(t, err, proxyConflict)
		assert.Equal(t, ctrl.Result{}, res)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, f.stored(t).Finalizers)
	})

	t.Run("a delete still failing more than an hour after the deletion is given up on, and the job goes", func(t *testing.T) {
		// A job outside the applications' namespace, so the log has to name
		// the application's namespace rather than the job's. It was deleted an
		// hour and a minute ago, which with the case above pins the give-up to
		// the hour.
		job := deletedSparkJob(launchedSparkJob(), 61*time.Minute)
		job.Namespace = "test-project"
		key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
		f := newReconcileFixture(t, job)
		var stored v2pb.SparkJob
		require.NoError(t, f.r.Get(context.Background(), key, &stored))
		deletedAt := stored.GetDeletionTimestamp().UTC().Format(time.RFC3339)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		injected := errors.New("member cluster unreachable")
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(injected)

		res, err := f.r.Reconcile(log.IntoContext(context.Background(), f.logger), ctrl.Request{NamespacedName: key})

		require.NoError(t, err, "a delete the controller gave up on is not retried")
		assert.Equal(t, ctrl.Result{}, res)
		err = f.r.Get(context.Background(), key, &v2pb.SparkJob{})
		require.True(t, apiErrors.IsNotFound(err), "the SparkJob must be gone, got %v", err)
		giveUps := f.logs.FilterMessage(giveUpMessage).All()
		require.Len(t, giveUps, 1, "the give-up is logged once, for an operator to delete the application")
		assert.Equal(t, zapcore.ErrorLevel, giveUps[0].Level)
		assert.Equal(t, map[string]interface{}{
			"cluster":           testClusterName,
			"spark_application": "default/" + sparkJobName,
			"deleted_at":        deletedAt,
			"error":             injected.Error(),
		}, giveUps[0].ContextMap())
	})

	t.Run("a cluster missing from the cache with no Cluster object is taken as deregistered, and the job goes", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), time.Minute))
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		// No DeleteJob is declared: there is no cluster to call.

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		f.requireGone(t)
	})

	t.Run("a cluster missing from the cache whose Cluster object exists has the SparkApplication deleted through it, then the job goes", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), time.Minute), registeredCluster())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), isRegisteredCluster{}).Return(nil).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		f.requireGone(t)
	})

	t.Run("a failed delete through the Cluster object keeps the finalizer and is returned for a retry", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), time.Minute), registeredCluster())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		injected := errors.New("member cluster unreachable")
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), isRegisteredCluster{}).Return(injected)
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers)
	})

	t.Run("a Cluster object that cannot be read keeps the finalizer and is returned for a retry", func(t *testing.T) {
		injected := errors.New("cluster read failed")
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			Get: failClusterReads(injected),
		}, deletedSparkJob(launchedSparkJob(), time.Minute), registeredCluster())
		f.cache.EXPECT().GetCluster(testClusterName).Return(nil)
		// No DeleteJob is declared: nothing is called while the cluster is unknown.
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers)
	})

	t.Run("a job marked only with the deleting annotation is torn down, then loses the finalizer", func(t *testing.T) {
		job := heldLaunchedSparkJob()
		job.Annotations = map[string]string{api.DeletingAnnotation: "true"}
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
		before := f.stored(t).Status

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Empty(t, stored.Finalizers, "nothing is left for the finalizer to hold the job for")
		assert.Nil(t, stored.GetDeletionTimestamp())
		assert.Equal(t, before, stored.Status, "nothing is written to the status of a job being deleted")
	})

	t.Run("a job marked only with the deleting annotation keeps the finalizer while its delete fails", func(t *testing.T) {
		job := heldLaunchedSparkJob()
		job.Annotations = map[string]string{api.DeletingAnnotation: "true"}
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		injected := errors.New("member cluster unreachable")
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(injected)

		res, err := f.reconcile()

		require.ErrorIs(t, err, injected)
		assert.Equal(t, ctrl.Result{}, res)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, f.stored(t).Finalizers)
	})
}

// TestReconciler_Reconcile_FreezeRemovesTheCleanupFinalizer covers the freeze
// taking the cleanup finalizer off in its own write: with the job's outcome
// recorded there is nothing left to stop, so the job's deletion is not held up.
// A finalizer of somebody else's stays.
func TestReconciler_Reconcile_FreezeRemovesTheCleanupFinalizer(t *testing.T) {
	t.Run("a recorded outcome, in the freeze's one write", func(t *testing.T) {
		updates := 0
		job := scheduledSparkJob(
			testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
			testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE),
		)
		job.Finalizers = []string{sparkApplicationCleanupFinalizer, controllerTestFinalizer}
		f := newReconcileFixtureWith(t, &interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				return c.Update(ctx, obj, opts...)
			},
		}, job)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.True(t, apiutils.IsImmutable(stored))
		assert.Equal(t, []string{controllerTestFinalizer}, stored.Finalizers)
		assert.Equal(t, 1, updates, "the finalizer comes off in the write that freezes the job")
	})

	t.Run("a termination", func(t *testing.T) {
		job := heldLaunchedSparkJob()
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		require.NotNil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition))
		assert.True(t, apiutils.IsImmutable(stored))
		assert.Empty(t, stored.Finalizers)
	})

	t.Run("a recorded outcome on a job whose deletion is pending, which then goes", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(scheduledSparkJob(
			testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
			testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE),
		), time.Minute))
		// No DeleteJob is declared: a job with an outcome leaves its
		// application to the TTL.

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		f.requireGone(t)
	})
}

// TestReconciler_Reconcile_ImmutableJobLosesALeftoverCleanupFinalizer covers a
// frozen job that still carries the cleanup finalizer: the controller does
// nothing more for a frozen job, so the finalizer comes off, and nothing else
// is written or called. A finalizer of somebody else's stays.
func TestReconciler_Reconcile_ImmutableJobLosesALeftoverCleanupFinalizer(t *testing.T) {
	frozenJob := func() *v2pb.SparkJob {
		job := scheduledSparkJob(
			testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE),
			testCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE),
		)
		apiutils.MarkImmutable(job)
		return job
	}

	t.Run("the finalizer is removed and nothing else changes", func(t *testing.T) {
		job := frozenJob()
		job.Finalizers = []string{sparkApplicationCleanupFinalizer, controllerTestFinalizer}
		f := newReconcileFixture(t, job)
		before := f.stored(t)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		stored := f.stored(t)
		assert.Equal(t, []string{controllerTestFinalizer}, stored.Finalizers)
		assert.Equal(t, before.Annotations, stored.Annotations)
		assert.Equal(t, before.Status, stored.Status)
		assert.Empty(t, f.queue.enqueued)
	})

	t.Run("a frozen job whose deletion is pending goes", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(frozenJob(), time.Minute))

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		f.requireGone(t)
	})
}

// TestReconciler_Reconcile_DeletedBeforeLaunch covers a job deleted before it
// was launched. It never got the cleanup finalizer, so the delete removes it in
// one step, as it always did, and the reconcile that follows finds nothing to
// launch or tear down. No cache or federated-client call is declared, so any
// would fail the test.
func TestReconciler_Reconcile_DeletedBeforeLaunch(t *testing.T) {
	f := newReconcileFixture(t, scheduledSparkJob())
	require.NoError(t, f.r.Delete(context.Background(), f.stored(t)))
	f.requireGone(t)

	res, err := f.reconcile()

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
}

// TestReconciler_Reconcile_Probe covers the safety net for a launched job with
// no recorded outcome: the reconcile reads the SparkApplication's status
// straight from the member cluster and records an outcome the watch missed.
func TestReconciler_Reconcile_Probe(t *testing.T) {
	gone := map[string]error{
		"not found": apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName),
		// How the federated client reports a missing application, and also an
		// application under the job's name that is not the job's own.
		"not found, wrapped by the federated client": fmt.Errorf("get spark application %q status: %w",
			sparkJobName, apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName)),
	}
	for name, probeErr := range gone {
		t.Run("a SparkApplication that is gone is recorded as deleted, not killed: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, probeErr)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			stored := f.stored(t)
			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, "SparkApplicationDeleted", succeeded.Reason)
			assert.Contains(t, succeeded.Message, testClusterName)
			assert.Contains(t, succeeded.Message, "last observed state: RUNNING")
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition),
				"Uniflow reads Killed=TRUE as do-not-retry, and an application that vanished may be retried")

			// The next reconcile freezes the job, without another remote call.
			res, err = f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, res)
			assert.True(t, apiutils.IsImmutable(f.stored(t)))
		})
	}

	terminal := []struct {
		name    string
		status  apipb.ConditionStatus
		reason  string
		message string
	}{
		{name: "completed", status: apipb.CONDITION_STATUS_TRUE, reason: "COMPLETED"},
		{name: "failed", status: apipb.CONDITION_STATUS_FALSE, reason: "FAILED", message: "driver container failed with exit code 1"},
	}
	for _, tc := range terminal {
		t.Run("a terminal status the watch missed is applied: "+tc.name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(probedSparkStatus(tc.status, tc.reason, tc.message), nil)

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
			stored := f.stored(t)
			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, tc.status, succeeded.Status)
			assert.Equal(t, tc.reason, succeeded.Reason)
			assert.Equal(t, tc.message, succeeded.Message)
			assert.Equal(t, "spark-application-1", stored.Status.ApplicationId, "the status is merged as the watch merges it")
			for _, owned := range []string{constants.EnqueuedCondition, constants.ScheduledCondition, constants.LaunchedCondition} {
				assert.NotNil(t, findCondition(stored.Status.StatusConditions, owned), "the merge must keep %s", owned)
			}
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition))

			// The next reconcile freezes the job, without another remote call.
			_, err = f.reconcile()

			require.NoError(t, err)
			assert.True(t, apiutils.IsImmutable(f.stored(t)))
		})
	}

	t.Run("a terminal status without the application's identifiers keeps the stored ones", func(t *testing.T) {
		job := launchedSparkJob()
		// The watch recorded both once the driver was up.
		job.Status.ApplicationId = "spark-application-1"
		job.Status.JobUrl = "http://driver/ui"
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		probed := probedSparkStatus(apipb.CONDITION_STATUS_FALSE, "FAILED", "driver pod was evicted")
		probed.Spark.ApplicationId = ""
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(probed, nil)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, res)
		stored := f.stored(t)
		succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
		assert.Equal(t, "FAILED", succeeded.Reason)
		assert.Equal(t, "spark-application-1", stored.Status.ApplicationId,
			"a probed status with no ApplicationId keeps the stored one, as the watch does")
		assert.Equal(t, "http://driver/ui", stored.Status.JobUrl,
			"a probed status with no JobUrl keeps the stored one, as the watch does")
	})

	nothing := map[string]struct {
		status *matypes.JobStatus
		err    error
	}{
		"the application is still running":        {status: probedSparkStatus(apipb.CONDITION_STATUS_UNKNOWN, "RUNNING", "")},
		"the status carries no outcome":           {status: &matypes.JobStatus{Spark: &v2pb.SparkJobStatus{}}},
		"the member cluster is unavailable":       {err: apiErrors.NewServiceUnavailable("etcdserver: leader changed")},
		"the cluster's client could not be built": {err: errors.New("get client for cluster err:spark client not configured")},
		// Not proof that the application is gone: it may still be running.
		"a 404 whose body is not a Kubernetes status": {err: fmt.Errorf("get spark application %q status: %w",
			sparkJobName, unstructuredNotFound())},
	}
	for name, tc := range nothing {
		t.Run("nothing is recorded and the probe runs again in five minutes: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.status, tc.err)
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err, "a probe that learns nothing is not an error: the next one tries again")
			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Minute}, res)
			assert.Equal(t, before, f.stored(t).ResourceVersion)
		})
	}

	raced := []struct {
		name          string
		race          func(t *testing.T, f *reconcileFixture)
		wantSucceeded apipb.ConditionStatus
	}{
		{
			name:          "the watch recorded the outcome",
			race:          func(t *testing.T, f *reconcileFixture) { recordOutcome(t, f, apipb.CONDITION_STATUS_TRUE, "COMPLETED") },
			wantSucceeded: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name: "a termination was requested",
			race: func(t *testing.T, f *reconcileFixture) {
				current := f.stored(t)
				current.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
				require.NoError(t, f.r.Update(context.Background(), current))
			},
			wantSucceeded: apipb.CONDITION_STATUS_UNKNOWN,
		},
		{
			// The API server's delete path marks the job and leaves the delete
			// to the ingester.
			name: "the job is being deleted",
			race: func(t *testing.T, f *reconcileFixture) {
				current := f.stored(t)
				current.Annotations = map[string]string{api.DeletingAnnotation: "true"}
				require.NoError(t, f.r.Update(context.Background(), current))
			},
			wantSucceeded: apipb.CONDITION_STATUS_UNKNOWN,
		},
	}
	for _, tc := range raced {
		t.Run("nothing is written over what changed since the probe's read: "+tc.name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, runtime.Object, *v2pb.Cluster) (*matypes.JobStatus, error) {
					tc.race(t, f)
					return nil, apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName)
				})

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Minute}, res)
			stored := f.stored(t)
			succeeded := findCondition(stored.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, tc.wantSucceeded, succeeded.Status)
			assert.NotEqual(t, reasonSparkApplicationDeleted, succeeded.Reason, "the probe's result must not be written")
			assert.Nil(t, findCondition(stored.Status.StatusConditions, constants.KilledCondition))
		})
	}
}

// TestReconciler_Reconcile_ProbeWaitsForAQuietStatus covers when the probe
// runs: only once the job's status has gone sparkStatusProbeInterval without a
// change. A condition written more recently shows the watch is delivering, so
// the reconcile asks the member cluster nothing and requeues for the rest of
// the interval, counted from the newest condition. GetJobStatus is declared
// only where a probe is expected, so an early one fails the test.
func TestReconciler_Reconcile_ProbeWaitsForAQuietStatus(t *testing.T) {
	t.Run("a condition written a minute ago: no probe until the interval has passed since", func(t *testing.T) {
		job := launchedSparkJob()
		// The watch recorded the application as running a minute ago.
		written := time.Now().Add(-time.Minute).Unix()
		findCondition(job.Status.StatusConditions, constants.SucceededCondition).LastUpdatedTimestamp = written
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		before := f.stored(t).ResourceVersion

		quietBefore := time.Since(time.Unix(written, 0))
		res, err := f.reconcile()
		quietAfter := time.Since(time.Unix(written, 0))

		require.NoError(t, err)
		// The interval less how long the status has been quiet, which the
		// reconcile measured between these two readings: a little under four
		// minutes.
		assert.LessOrEqual(t, res.RequeueAfter, sparkStatusProbeInterval-quietBefore)
		assert.GreaterOrEqual(t, res.RequeueAfter, sparkStatusProbeInterval-quietAfter)
		assert.Equal(t, before, f.stored(t).ResourceVersion, "a reconcile that does not probe writes nothing")
	})

	t.Run("a condition written in the future: no probe for a whole interval", func(t *testing.T) {
		job := launchedSparkJob()
		findCondition(job.Status.StatusConditions, constants.SucceededCondition).LastUpdatedTimestamp =
			time.Now().Add(10 * time.Minute).Unix()
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res,
			"a timestamp ahead of the clock counts as written now, never as a longer wait")
	})

	t.Run("every condition older than the interval: the probe runs", func(t *testing.T) {
		job := launchedSparkJob()
		stampConditions(job, time.Now().Add(-sparkStatusProbeInterval-time.Second))
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(probedSparkStatus(apipb.CONDITION_STATUS_UNKNOWN, "RUNNING", ""), nil).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
	})

	t.Run("the reconcile the launch's own write triggers does not probe", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil)

		res, err := f.reconcile()
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)

		// The Launched write's watch event reconciles the job again at once.
		res, err = f.reconcile()

		require.NoError(t, err)
		assert.LessOrEqual(t, res.RequeueAfter, sparkStatusProbeInterval)
		assert.Greater(t, res.RequeueAfter, sparkStatusProbeInterval-2*time.Second,
			"the first probe comes an interval after the launch")
	})
}

// TestSparkStatusQuietWait pins the arithmetic of the probe's wait for a quiet
// status, on a fixed clock: the interval less the age of the newest condition,
// wherever that condition sits in the list, nothing once the interval has
// passed, and never more than the interval.
func TestSparkStatusQuietWait(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ago := func(d time.Duration) int64 { return now.Add(-d).Unix() }

	tests := []struct {
		name  string
		stamp []int64
		want  time.Duration
	}{
		{name: "written a minute ago", stamp: []int64{ago(time.Minute)}, want: 4 * time.Minute},
		{name: "written just now", stamp: []int64{ago(0)}, want: sparkStatusProbeInterval},
		{
			name:  "the newest condition counts, wherever it is in the list",
			stamp: []int64{ago(time.Hour), ago(30 * time.Second), ago(20 * time.Minute)},
			want:  4*time.Minute + 30*time.Second,
		},
		{name: "quiet for exactly the interval", stamp: []int64{ago(sparkStatusProbeInterval)}, want: 0},
		{name: "quiet for longer", stamp: []int64{ago(time.Hour), ago(6 * time.Minute)}, want: 0},
		{name: "written in the future", stamp: []int64{ago(time.Hour), ago(-time.Hour)}, want: sparkStatusProbeInterval},
		{name: "never stamped", stamp: []int64{0, 0}, want: 0},
		{name: "no conditions", stamp: nil, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := testSparkJob()
			for _, stamp := range tc.stamp {
				condition := testCondition(fmt.Sprintf("Condition%d", len(job.Status.StatusConditions)), apipb.CONDITION_STATUS_TRUE)
				condition.LastUpdatedTimestamp = stamp
				job.Status.StatusConditions = append(job.Status.StatusConditions, condition)
			}

			assert.Equal(t, tc.want, sparkStatusQuietWait(job, now))
		})
	}
}

// TestReconciler_Reconcile_ProbeHoldsAClusterAfterAFailedProbe covers the hold
// a failed probe puts on its cluster: once a probe has failed with anything but
// a NotFound that carries a Kubernetes Status, no probe goes to that cluster
// for sparkStatusProbeInterval, and a job there whose probe is due is
// requeued for another interval without a remote call. A cluster that has
// stopped answering then stalls one wave of calls per hold, no more than one
// per worker, rather than one probe per job. GetJobStatus is declared only
// where a probe is expected, so a probe the hold should have stopped fails the
// test. TestReconciler_Reconcile_ClusterHold covers what the hold does to
// launches, terminations and deletions.
func TestReconciler_Reconcile_ProbeHoldsAClusterAfterAFailedProbe(t *testing.T) {
	const (
		secondSparkJobName = "test-spark-job-2"
		otherClusterName   = "member-cluster-2"
		holdMessage        = "could not probe the spark application, holding calls to its cluster"
	)
	// A request to a member cluster whose network drops it runs into the
	// probe's deadline.
	timeout := fmt.Errorf("get spark application %q status: %w", sparkJobName,
		&url.Error{Op: "Get", URL: memberAPIURL + "/" + sparkJobName, Err: context.DeadlineExceeded})
	running := probedSparkStatus(apipb.CONDITION_STATUS_UNKNOWN, "RUNNING", "")

	holding := map[string]error{
		"a timeout":                               timeout,
		"the member cluster is unavailable":       apiErrors.NewServiceUnavailable("etcdserver: leader changed"),
		"the cluster's client could not be built": errors.New("get client for cluster err:spark client not configured"),
		"a 404 whose body is not a Kubernetes status": fmt.Errorf("get spark application %q status: %w",
			sparkJobName, unstructuredNotFound()),
	}
	for name, probeErr := range holding {
		t.Run("a failed probe holds the next probe to the same cluster: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob(), launchedSparkJobOn(secondSparkJobName, testClusterName))
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, probeErr).Times(1)

			failedAfter := time.Now()
			res, err := f.reconcile()
			failedBefore := time.Now()

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
			until, held := f.r.clusterHeldUntil(testClusterName, time.Now())
			require.True(t, held, "a failed probe holds calls to its cluster")
			assert.False(t, until.Before(failedAfter.Add(sparkStatusProbeInterval)), "the hold lasts an interval from the failure")
			assert.False(t, until.After(failedBefore.Add(sparkStatusProbeInterval)), "the hold lasts an interval from the failure")
			holdLogs := f.logs.FilterMessage(holdMessage).All()
			require.Len(t, holdLogs, 1, "the start of a hold is logged once")
			assert.Equal(t, zapcore.InfoLevel, holdLogs[0].Level)
			assert.Equal(t, map[string]interface{}{
				"cluster": testClusterName,
				"until":   until.Format(time.RFC3339),
				"error":   probeErr.Error(),
			}, holdLogs[0].ContextMap())

			// The second job's probe is due too, but its cluster is on hold.
			before := f.storedNamed(t, secondSparkJobName).ResourceVersion

			res, err = f.reconcileNamed(secondSparkJobName)

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Minute}, res)
			assert.Equal(t, before, f.storedNamed(t, secondSparkJobName).ResourceVersion)
			assert.Len(t, f.logs.FilterMessage(holdMessage).All(), 1, "a held probe starts no hold of its own")
		})
	}

	t.Run("once the hold has ended, the next probe goes out", func(t *testing.T) {
		f := newReconcileFixture(t, launchedSparkJob(), launchedSparkJobOn(secondSparkJobName, testClusterName))
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(3)
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, timeout).Times(1)

		_, err := f.reconcile()
		require.NoError(t, err)
		res, err := f.reconcileNamed(secondSparkJobName)
		require.NoError(t, err)
		require.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)

		// The hold ended a second ago.
		f.r.clusterHolds[testClusterName] = time.Now().Add(-time.Second)
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(running, nil).Times(1)

		res, err = f.reconcileNamed(secondSparkJobName)

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
	})

	answering := map[string]struct {
		status *matypes.JobStatus
		err    error
	}{
		"the application is gone": {err: apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName)},
		"the application is gone, wrapped by the federated client": {err: fmt.Errorf("get spark application %q status: %w",
			sparkJobName, apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName))},
		"the application is still running": {status: running},
	}
	for name, tc := range answering {
		t.Run("a probe the cluster answers starts no hold: "+name, func(t *testing.T) {
			f := newReconcileFixture(t, launchedSparkJob(), launchedSparkJobOn(secondSparkJobName, testClusterName))
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.status, tc.err).Times(1)

			_, err := f.reconcile()

			require.NoError(t, err)
			_, held := f.r.clusterHeldUntil(testClusterName, time.Now())
			assert.False(t, held)

			// The second job on the same cluster is probed as usual.
			f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(running, nil).Times(1)

			res, err := f.reconcileNamed(secondSparkJobName)

			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		})
	}

	t.Run("a hold on one cluster leaves the jobs on another alone", func(t *testing.T) {
		f := newReconcileFixture(t, launchedSparkJob(), launchedSparkJobOn(secondSparkJobName, otherClusterName))
		otherCluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: otherClusterName}}
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.cache.EXPECT().GetCluster(otherClusterName).Return(otherCluster)
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, timeout).Times(1)

		_, err := f.reconcile()
		require.NoError(t, err)
		_, held := f.r.clusterHeldUntil(testClusterName, time.Now())
		require.True(t, held)

		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), otherCluster).Return(running, nil).Times(1)

		res, err := f.reconcileNamed(secondSparkJobName)

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		_, held = f.r.clusterHeldUntil(otherClusterName, time.Now())
		assert.False(t, held)
	})
}

// clusterHoldPath is one of the reconciles besides the probe that call the
// job's cluster, starting from a job whose next reconcile makes that call: a
// launch creates the SparkApplication, a termination and a deletion delete it.
type clusterHoldPath struct {
	name string
	// operation is how the reconcile names its call while the cluster is held.
	operation string
	job       func() *v2pb.SparkJob
	// expectCall declares the path's remote call, failing with err.
	expectCall func(f *reconcileFixture, err error) *gomock.Call
}

// clusterHoldPaths returns the launch, the termination and the deletion.
func clusterHoldPaths() []clusterHoldPath {
	expectDelete := func(f *reconcileFixture, err error) *gomock.Call {
		return f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(err)
	}
	return []clusterHoldPath{
		{
			name:      "launch",
			operation: "create",
			job:       func() *v2pb.SparkJob { return scheduledSparkJob() },
			expectCall: func(f *reconcileFixture, err error) *gomock.Call {
				return f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(err)
			},
		},
		{
			name:      "termination",
			operation: "terminate",
			job: func() *v2pb.SparkJob {
				job := heldLaunchedSparkJob()
				job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
				return job
			},
			expectCall: expectDelete,
		},
		{
			name:       "deletion",
			operation:  "delete",
			job:        func() *v2pb.SparkJob { return deletedSparkJob(launchedSparkJob(), time.Minute) },
			expectCall: expectDelete,
		},
	}
}

// TestReconciler_Reconcile_ClusterHold covers the hold on a member cluster as
// the launch, the termination and the deletion meet it. While the job's
// cluster is held, each of them calls nothing and writes nothing, so no cleanup
// finalizer is added or removed, no Killed condition is recorded and the record
// of a retried creation is not rewritten, and the job is requeued for the rest
// of the hold. A create or a delete that runs out of time starts a hold, and
// one that fails fast does not. Each remote call is declared exactly as often
// as it is expected, so a call the hold should have stopped fails the test.
// TestReconciler_Reconcile_ProbeHoldsAClusterAfterAFailedProbe covers the
// probe.
func TestReconciler_Reconcile_ClusterHold(t *testing.T) {
	const (
		secondSparkJobName = "test-spark-job-2"
		otherClusterName   = "member-cluster-2"
		waitMessage        = "calls to the spark job's cluster are on hold, waiting for the hold to end"
		holdMessage        = "the spark job's cluster did not answer in time, holding calls to it"
	)
	timeouts := map[string]error{
		"the call's deadline passed": fmt.Errorf("member cluster call err:%w", transportError(context.DeadlineExceeded)),
		"the connection timed out": fmt.Errorf("member cluster call err:%w",
			transportError(&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded})),
	}
	fastFailures := map[string]error{
		"the API server is unavailable": apiErrors.NewServiceUnavailable("etcdserver: leader changed"),
		"the connection was refused":    transportError(dialError(syscall.ECONNREFUSED)),
	}

	for _, path := range clusterHoldPaths() {
		t.Run(path.name+": a held cluster gets no call, and nothing is written", func(t *testing.T) {
			f := newReconcileFixture(t, path.job())
			f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
			heldUntil := time.Now().Add(time.Minute)
			f.r.clusterHolds = map[string]time.Time{testClusterName: heldUntil}
			before := f.stored(t).ResourceVersion

			res, err := f.reconcile()

			require.NoError(t, err)
			assert.Greater(t, res.RequeueAfter, 50*time.Second, "the job waits out the rest of the hold")
			assert.LessOrEqual(t, res.RequeueAfter, time.Minute, "the job waits out the rest of the hold")
			assert.Equal(t, before, f.stored(t).ResourceVersion, "nothing may be written while the cluster is held")
			waits := f.logs.FilterMessage(waitMessage).All()
			require.Len(t, waits, 1)
			assert.Equal(t, map[string]interface{}{
				"cluster":   testClusterName,
				"until":     heldUntil.Format(time.RFC3339),
				"operation": path.operation,
			}, waits[0].ContextMap())
		})

		for name, callErr := range timeouts {
			t.Run(path.name+": a call that runs out of time holds the cluster, and the next reconcile makes none: "+name, func(t *testing.T) {
				f := newReconcileFixture(t, path.job())
				f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
				path.expectCall(f, callErr).Times(1)

				failedAfter := time.Now()
				res, err := f.reconcile()
				failedBefore := time.Now()

				require.ErrorIs(t, err, callErr)
				assert.Equal(t, ctrl.Result{}, res)
				until, held := f.r.clusterHeldUntil(testClusterName, time.Now())
				require.True(t, held, "a call that ran out of time holds its cluster")
				assert.False(t, until.Before(failedAfter.Add(sparkStatusProbeInterval)), "the hold lasts an interval from the failure")
				assert.False(t, until.After(failedBefore.Add(sparkStatusProbeInterval)), "the hold lasts an interval from the failure")
				holds := f.logs.FilterMessage(holdMessage).All()
				require.Len(t, holds, 1, "the start of a hold is logged once")
				assert.Equal(t, zapcore.InfoLevel, holds[0].Level)
				assert.Equal(t, map[string]interface{}{
					"cluster": testClusterName,
					"until":   until.Format(time.RFC3339),
					"error":   callErr.Error(),
				}, holds[0].ContextMap())
				before := f.stored(t).ResourceVersion

				res, err = f.reconcile()

				require.NoError(t, err)
				assert.Greater(t, res.RequeueAfter, time.Duration(0), "the job waits out the rest of the hold")
				assert.LessOrEqual(t, res.RequeueAfter, sparkStatusProbeInterval, "the job waits out the rest of the hold")
				assert.Equal(t, before, f.stored(t).ResourceVersion, "nothing may be written while the cluster is held")
			})
		}

		for name, callErr := range fastFailures {
			t.Run(path.name+": a call that fails fast holds nothing, and the next reconcile calls again: "+name, func(t *testing.T) {
				f := newReconcileFixture(t, path.job())
				f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
				path.expectCall(f, callErr).Times(2)

				_, err := f.reconcile()

				require.ErrorIs(t, err, callErr)
				_, held := f.r.clusterHeldUntil(testClusterName, time.Now())
				assert.False(t, held, "a call that failed fast holds nothing")
				assert.Empty(t, f.logs.FilterMessage(holdMessage).All())

				_, err = f.reconcile()

				require.ErrorIs(t, err, callErr, "the next reconcile calls the cluster again")
			})
		}
	}

	t.Run("a held cluster puts off the give-up on a deletion more than an hour old until a delete has failed", func(t *testing.T) {
		f := newReconcileFixture(t, deletedSparkJob(launchedSparkJob(), 2*time.Hour))
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		f.r.clusterHolds = map[string]time.Time{testClusterName: time.Now().Add(time.Minute)}
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Greater(t, res.RequeueAfter, 50*time.Second, "the job waits out the rest of the hold")
		assert.LessOrEqual(t, res.RequeueAfter, time.Minute, "the job waits out the rest of the hold")
		stored := f.stored(t)
		assert.Equal(t, before, stored.ResourceVersion)
		assert.Equal(t, []string{sparkApplicationCleanupFinalizer}, stored.Finalizers,
			"the controller gives up only after a delete has failed")

		// The hold ended a second ago: the delete goes out, fails, and the
		// controller gives up.
		f.r.clusterHolds[testClusterName] = time.Now().Add(-time.Second)
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("member cluster unreachable")).Times(1)

		res, err = f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
		f.requireGone(t)
	})

	t.Run("a failed probe holds the launches on its cluster", func(t *testing.T) {
		toLaunch := scheduledSparkJob()
		toLaunch.Name = secondSparkJobName
		f := newReconcileFixture(t, launchedSparkJob(), toLaunch)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, apiErrors.NewServiceUnavailable("etcdserver: leader changed")).Times(1)
		// No CreateJob is declared: a launch would fail the test.

		_, err := f.reconcile()
		require.NoError(t, err)
		before := f.storedNamed(t, secondSparkJobName).ResourceVersion

		res, err := f.reconcileNamed(secondSparkJobName)

		require.NoError(t, err)
		assert.Greater(t, res.RequeueAfter, time.Duration(0), "the job waits out the rest of the hold")
		assert.LessOrEqual(t, res.RequeueAfter, sparkStatusProbeInterval, "the job waits out the rest of the hold")
		assert.Equal(t, before, f.storedNamed(t, secondSparkJobName).ResourceVersion,
			"not even the cleanup finalizer is added while the cluster is held")
	})

	t.Run("a creation that ran out of time holds the probes on its cluster", func(t *testing.T) {
		toLaunch := scheduledSparkJob()
		toLaunch.Name = secondSparkJobName
		f := newReconcileFixture(t, launchedSparkJob(), toLaunch)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster()).Times(2)
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
			Return(transportError(context.DeadlineExceeded)).Times(1)
		// No GetJobStatus is declared: a probe would fail the test.

		_, err := f.reconcileNamed(secondSparkJobName)
		require.Error(t, err)
		before := f.stored(t).ResourceVersion

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
		assert.Equal(t, before, f.stored(t).ResourceVersion)
	})

	t.Run("a hold on another cluster leaves the calls to this one alone", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.r.clusterHolds = map[string]time.Time{otherClusterName: time.Now().Add(time.Minute)}
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).Return(nil).Times(1)

		res, err := f.reconcile()

		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, res)
	})
}

// TestRecordSparkApplicationGone pins the outcome recorded for a
// SparkApplication that disappeared before it reported one: a failure
// naming the cluster and, when known, the last state, and no Killed condition.
func TestRecordSparkApplicationGone(t *testing.T) {
	tests := []struct {
		name        string
		lastState   string
		wantMessage string
	}{
		{
			name:        "with the last observed state",
			lastState:   "RUNNING",
			wantMessage: `the SparkApplication on cluster "member-cluster-1" was deleted before it reported an outcome; last observed state: RUNNING`,
		},
		{
			name:        "without one",
			lastState:   "",
			wantMessage: `the SparkApplication on cluster "member-cluster-1" was deleted before it reported an outcome`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := launchedSparkJob()

			recordSparkApplicationGone(job, tc.lastState)

			succeeded := findCondition(job.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, reasonSparkApplicationDeleted, succeeded.Reason)
			assert.Equal(t, tc.wantMessage, succeeded.Message)
			assert.Nil(t, findCondition(job.Status.StatusConditions, constants.KilledCondition))
			assert.Len(t, job.Status.StatusConditions, 4, "the Succeeded condition is updated in place, not added twice")
		})
	}
}

// TestReconciler_RemoteCallsHaveADeadline covers the deadline on remote calls:
// every call to a member cluster runs under remoteCallTimeout, so one that
// hangs keeps its worker no longer than that.
func TestReconciler_RemoteCallsHaveADeadline(t *testing.T) {
	t.Run("CreateJob", func(t *testing.T) {
		f := newReconcileFixture(t, scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _, _ runtime.Object, _ *v2pb.Cluster) error {
				requireRemoteCallDeadline(t, ctx)
				return nil
			})

		_, err := f.reconcile()

		require.NoError(t, err)
	})

	t.Run("DeleteJob on termination", func(t *testing.T) {
		job := scheduledSparkJob(testCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE))
		job.Spec.Termination = testTermination(v2pb.TERMINATION_TYPE_FAILED)
		f := newReconcileFixture(t, job)
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ runtime.Object, _ *v2pb.Cluster) error {
				requireRemoteCallDeadline(t, ctx)
				return nil
			})

		_, err := f.reconcile()

		require.NoError(t, err)
	})

	t.Run("DeleteJob on deletion", func(t *testing.T) {
		f := newDeletingFixture(t, "deletion timestamp", scheduledSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().DeleteJob(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ runtime.Object, _ *v2pb.Cluster) error {
				requireRemoteCallDeadline(t, ctx)
				return nil
			})

		_, err := f.reconcile()

		require.NoError(t, err)
	})

	t.Run("GetJobStatus", func(t *testing.T) {
		f := newReconcileFixture(t, launchedSparkJob())
		f.cache.EXPECT().GetCluster(testClusterName).Return(testCluster())
		f.federated.EXPECT().GetJobStatus(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ runtime.Object, _ *v2pb.Cluster) (*matypes.JobStatus, error) {
				requireRemoteCallDeadline(t, ctx)
				return probedSparkStatus(apipb.CONDITION_STATUS_UNKNOWN, "RUNNING", ""), nil
			})

		_, err := f.reconcile()

		require.NoError(t, err)
	})
}

// TestReconciler_Reconcile_EnqueueToLaunch walks the whole admission path: a
// fresh SparkJob is enqueued, the scheduler writes its assignment, and the next
// reconcile launches it on exactly that cluster.
func TestReconciler_Reconcile_EnqueueToLaunch(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, testSparkJob())

	res, err := f.reconcile()
	require.NoError(t, err)
	assert.Equal(t, requeueAfter, res.RequeueAfter)
	require.Len(t, f.queue.enqueued, 1)

	// Stand in for the scheduler: assign the queued job to a member cluster.
	scheduled := f.stored(t)
	scheduled.Status.Assignment = &v2pb.AssignmentInfo{Cluster: testClusterName}
	scheduled.Status.StatusConditions = append(scheduled.Status.StatusConditions,
		testCondition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE))
	require.NoError(t, f.r.Status().Update(ctx, scheduled))

	assigned := testCluster()
	f.cache.EXPECT().GetCluster(testClusterName).Return(assigned)
	f.federated.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Nil(), assigned).Return(nil).Times(1)

	res, err = f.reconcile()

	require.NoError(t, err)
	assert.Equal(t, sparkStatusProbeInterval, res.RequeueAfter)
	launched := findCondition(f.stored(t).Status.StatusConditions, constants.LaunchedCondition)
	require.NotNil(t, launched, "Launched condition should be set")
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
}

// recordingManager is a real controller-runtime manager that also remembers
// every runnable handed to it, so a test can see what Register added without
// starting anything.
type recordingManager struct {
	manager.Manager
	added []manager.Runnable
}

func (m *recordingManager) Add(runnable manager.Runnable) error {
	m.added = append(m.added, runnable)
	return m.Manager.Add(runnable)
}

// TestReconciler_Register covers what Register hands the manager. The
// federated watch is added as a leader-only runnable, so the manager starts it
// once this instance is elected and stops it through the context it passes in,
// instead of Register starting it on a goroutine of its own with a context
// nothing cancels. The controller is added with ten workers, so a call to a
// member cluster that has stopped answering keeps one of them waiting rather
// than every SparkJob.
func TestReconciler_Register(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	skipNameValidation := true
	mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
		Scheme: scheme,
		// The controller's name is process-wide; nothing here starts it.
		Controller: config.Controller{SkipNameValidation: &skipNameValidation},
	})
	require.NoError(t, err)
	recording := &recordingManager{Manager: mgr}

	mockCtrl := gomock.NewController(t)
	r := NewReconciler(
		logr.Discard(),
		fake.NewClientBuilder().WithScheme(scheme).Build(),
		env.Context{},
		&mockSchedulerQueue{},
		clientmocks.NewMockFederatedClient(mockCtrl),
		// No expectations: a watch that synced would ask the cache for clusters.
		clustermocks.NewMockRegisteredClustersCache(mockCtrl),
		typesmocks.NewMockMapper(mockCtrl),
		tally.NoopScope,
	)

	require.NoError(t, r.Register(recording))

	var watches []controllerutil.LeaderOnlyRunnable
	for _, runnable := range recording.added {
		if watch, ok := runnable.(controllerutil.LeaderOnlyRunnable); ok {
			watches = append(watches, watch)
		}
	}
	require.Len(t, watches, 1, "the federated watch must be added to the manager as a leader-only runnable")
	assert.True(t, watches[0].NeedLeaderElection())

	// controller-runtime adds the controller to the manager as its own
	// Controller type, which lives in an internal package, so its worker count
	// is read through reflection, from the exported field the type keeps it in.
	var workers []int64
	for _, runnable := range recording.added {
		value := reflect.ValueOf(runnable)
		if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
			continue
		}
		if field := value.Elem().FieldByName("MaxConcurrentReconciles"); field.IsValid() && field.CanInt() {
			workers = append(workers, field.Int())
		}
	}
	require.Len(t, workers, 1, "the SparkJob controller must be added to the manager")
	assert.EqualValues(t, 10, workers[0], "the controller runs ten workers")

	// The watch runs on the context the manager passes in: started with one
	// that is already cancelled, it returns at once, without syncing a cluster.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- watches[0].Start(ctx) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the watch did not stop when its context was cancelled")
	}
}

// findCondition returns the condition of the given type, or nil if not present.
func findCondition(conditions []*apipb.Condition, conditionType string) *apipb.Condition {
	for _, cond := range conditions {
		if cond.Type == conditionType {
			return cond
		}
	}
	return nil
}
