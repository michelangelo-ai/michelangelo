package kueue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/kueuemocks"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// countingHandler records every status write and can serve one stale
// RayCluster read, standing in for a lagging cache.
type countingHandler struct {
	api.Handler

	mu     sync.Mutex
	writes []client.Object
	stale  *v2pb.RayCluster
}

func (h *countingHandler) Get(ctx context.Context, namespace string, name string, opts *metav1.GetOptions, obj client.Object) error {
	h.mu.Lock()
	stale := h.stale
	h.stale = nil
	h.mu.Unlock()
	if rc, ok := obj.(*v2pb.RayCluster); ok && stale != nil {
		stale.DeepCopyInto(rc)
		return nil
	}
	return h.Handler.Get(ctx, namespace, name, opts, obj)
}

func (h *countingHandler) UpdateStatus(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	h.mu.Lock()
	h.writes = append(h.writes, obj.DeepCopyObject().(client.Object))
	h.mu.Unlock()
	return h.Handler.UpdateStatus(ctx, obj, opts)
}

func (h *countingHandler) statusWrites() []client.Object {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]client.Object(nil), h.writes...)
}

func testRayCluster(labels map[string]string, assignedCluster string) *v2pb.RayCluster {
	rc := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-1",
			Namespace: "proj-ns",
			Labels:    labels,
		},
	}
	if assignedCluster != "" {
		rc.Status.Assignment = &v2pb.AssignmentInfo{Cluster: assignedCluster}
	}
	return rc
}

func kueueCluster(name string) *v2pb.Cluster {
	return &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
	}
}

func defaultCluster(name string) *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// newCountingHandler returns a handler over a fake client that stores the
// objects.
func newCountingHandler(t *testing.T, objs ...client.Object) *countingHandler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v2pb.RayCluster{}, &v2pb.SparkJob{}).
		Build()
	return &countingHandler{Handler: apiHandler.NewFakeAPIHandler(fakeClient)}
}

func newTestQueue(t *testing.T, strategy framework.AssignmentStrategy,
	clusters *clustermocks.MockRegisteredClustersCache, queues LocalQueues, cfg maconfig.SchedulerConfig,
	objs ...client.Object,
) (*KueueJobQueue, *countingHandler) {
	handler := newCountingHandler(t, objs...)

	q := NewKueueJobQueue(Params{
		Handler:            handler,
		AssignmentStrategy: strategy,
		ClusterCache:       clusters,
		LocalQueues:        queues,
		Config:             cfg,
		Scope:              tally.NewTestScope("test", nil),
		Logger:             logr.Discard(),
	})
	return q, handler
}

func setup(t *testing.T, rc *v2pb.RayCluster, strategy framework.AssignmentStrategy,
	clusters *clustermocks.MockRegisteredClustersCache, queues LocalQueues, cfg maconfig.SchedulerConfig,
) (*KueueJobQueue, *countingHandler, func() *v2pb.RayCluster) {
	q, handler := newTestQueue(t, strategy, clusters, queues, cfg, rc)
	readBack := func() *v2pb.RayCluster {
		return readRayCluster(t, handler, rc.Namespace, rc.Name)
	}
	return q, handler, readBack
}

func readRayCluster(t *testing.T, h *countingHandler, namespace, name string) *v2pb.RayCluster {
	got := &v2pb.RayCluster{}
	require.NoError(t, h.Handler.Get(context.Background(), namespace, name, &metav1.GetOptions{}, got))
	return got
}

func schedulableJob(rc *v2pb.RayCluster) matypes.SchedulableJob {
	return framework.BatchRayCluster{RayCluster: rc}
}

func projectLabels() map[string]string {
	return map[string]string{constants.ProjectNameLabelKey: "proj1"}
}

func kueueConfig() maconfig.SchedulerConfig {
	return maconfig.SchedulerConfig{Backend: "kueue"}
}

// assertScheduledTo asserts the recorded placement decision: the assignment
// and Scheduled=True/ClusterMatched naming the cluster.
func assertScheduledTo(t *testing.T, status *v2pb.RayClusterStatus, clusterName string) {
	t.Helper()
	assert.Equal(t, clusterName, status.Assignment.GetCluster())
	cond := findCondition(status.StatusConditions, constants.ScheduledCondition)
	require.NotNil(t, cond)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, cond.Status)
	assert.Equal(t, constants.ClusterMatched, cond.Reason)
	assert.Equal(t, "cluster "+clusterName, cond.Message)
}

// assertNotScheduled asserts that no placement decision was recorded.
func assertNotScheduled(t *testing.T, status *v2pb.RayClusterStatus) {
	t.Helper()
	cond := findCondition(status.StatusConditions, constants.ScheduledCondition)
	if cond != nil {
		assert.NotEqual(t, apipb.CONDITION_STATUS_TRUE, cond.Status)
	}
}

func TestEnqueueValidatesAndRecordsKueueTargetInOneWrite(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)

	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(true, nil)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	// One status write carries the whole decision.
	writes := handler.statusWrites()
	require.Len(t, writes, 1)
	written := writes[0].(*v2pb.RayCluster)
	assertScheduledTo(t, &written.Status, "kueue-a")
	enqueued := findCondition(written.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)

	assertScheduledTo(t, &readBack().Status, "kueue-a")
}

func TestEnqueueKeepsEnqueuedWhenAlreadyTrue(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")
	rc.Status.StatusConditions = []*apipb.Condition{{
		Type:                 constants.EnqueuedCondition,
		Status:               apipb.CONDITION_STATUS_TRUE,
		Reason:               constants.AddedToSchedulerQueue,
		LastUpdatedTimestamp: 12345,
	}}

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

	require.Len(t, handler.statusWrites(), 1)
	got := readBack()
	assertScheduledTo(t, &got.Status, "cluster-a")
	enqueued := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, int64(12345), enqueued.LastUpdatedTimestamp, "an Enqueued=True condition is left as is")
}

func TestEnqueueRecordsNonKueueTargetWithoutValidation(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)

	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))

	queues := kueuemocks.NewMockLocalQueues(g) // no Exists expectation: must not be called

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	require.Len(t, handler.statusWrites(), 1)
	assertScheduledTo(t, &readBack().Status, "cluster-a")
}

func TestEnqueueNoTargetWritesOnlyOnChange(t *testing.T) {
	tests := []struct {
		name   string
		expect func(*frameworkmocks.MockAssignmentEngine, *clustermocks.MockRegisteredClustersCache)
	}{
		{
			name: "strategy finds no cluster",
			expect: func(s *frameworkmocks.MockAssignmentEngine, _ *clustermocks.MockRegisteredClustersCache) {
				s.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, false, "no clusters", nil).Times(2)
			},
		},
		{
			name: "strategy picks an unregistered cluster",
			expect: func(s *frameworkmocks.MockAssignmentEngine, c *clustermocks.MockRegisteredClustersCache) {
				s.EXPECT().Select(gomock.Any(), gomock.Any()).
					Return(&v2pb.AssignmentInfo{Cluster: "gone"}, true, "", nil).Times(2)
				c.EXPECT().GetCluster("gone").Return(nil).Times(2)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := testRayCluster(projectLabels(), "")

			strategy := frameworkmocks.NewMockAssignmentEngine(g)
			clusters := clustermocks.NewMockRegisteredClustersCache(g)
			tt.expect(strategy, clusters)
			queues := kueuemocks.NewMockLocalQueues(g)

			q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
			require.Len(t, handler.statusWrites(), 1)

			got := readBack()
			assert.Empty(t, got.Status.Assignment.GetCluster())
			cond := findCondition(got.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, cond)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
			assert.Equal(t, constants.NoClustersFoundForAssignment, cond.Reason)
			// The job waits in the queue: the same write sets Enqueued True.
			enqueued := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)

			// The same outcome again is a fresh decision but no new write.
			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
			assert.Len(t, handler.statusWrites(), 1)
		})
	}
}

func TestEnqueueNoTargetAfterAFailedValidationSetsEnqueuedTrueOnce(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")
	rc.Status.StatusConditions = []*apipb.Condition{
		{Type: constants.EnqueuedCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.KueueQueueNotFound},
		{Type: constants.ScheduledCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.NoClustersFoundForAssignment},
	}
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).Return(nil, false, "no clusters", nil).Times(2)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	require.Len(t, handler.statusWrites(), 1, "Scheduled is unchanged, but Enqueued turns True")
	enqueued := findCondition(readBack().Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, enqueued)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)

	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Len(t, handler.statusWrites(), 1)
}

func TestEnqueueFailedValidationWritesEnqueuedFalseOncePerReason(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")
	rc.Status.StatusConditions = []*apipb.Condition{
		// The RayCluster controller no longer pre-writes Enqueued under this
		// backend; a True one comes from an earlier decision, such as one a
		// re-placement undid.
		{Type: constants.EnqueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.AddedToSchedulerQueue},
	}

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil).AnyTimes()
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target).AnyTimes()
	queues := kueuemocks.NewMockLocalQueues(g)
	gomock.InOrder(
		queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(false, nil).Times(3),
		queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(false, errors.New("connection refused")),
		queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(false, errors.New("i/o timeout")),
	)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	enqueued := func() *apipb.Condition {
		cond := findCondition(readBack().Status.StatusConditions, constants.EnqueuedCondition)
		require.NotNil(t, cond)
		return cond
	}

	// A failing validation returns an error every time, but writes
	// False/<reason> once across the retries.
	for range 3 {
		require.Error(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	}
	require.Len(t, handler.statusWrites(), 1)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, enqueued().Status)
	assert.Equal(t, constants.KueueQueueNotFound, enqueued().Reason)

	// A new reason is written once; a new message alone is not.
	require.Error(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	require.Len(t, handler.statusWrites(), 2)
	assert.Equal(t, constants.KueueQueueCheckFailed, enqueued().Reason)
	assert.Contains(t, enqueued().Message, "connection refused")
	require.Error(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Len(t, handler.statusWrites(), 2)
	assert.Contains(t, enqueued().Message, "connection refused")
}

func TestRecordNoTargetResult(t *testing.T) {
	condition := func(condType string, status apipb.ConditionStatus, reason string) *apipb.Condition {
		return &apipb.Condition{Type: condType, Status: status, Reason: reason, LastUpdatedTimestamp: 7}
	}
	noTarget := func() *apipb.Condition {
		return condition(constants.ScheduledCondition, apipb.CONDITION_STATUS_FALSE, constants.NoClustersFoundForAssignment)
	}
	enqueued := func() *apipb.Condition {
		return condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, constants.AddedToSchedulerQueue)
	}
	tests := []struct {
		name        string
		conditions  []*apipb.Condition
		wantChanged bool
	}{
		{name: "no conditions", wantChanged: true},
		{name: "Enqueued True, Scheduled Unknown", conditions: []*apipb.Condition{enqueued()}, wantChanged: true},
		{
			name:        "Scheduled already False/NoClustersFoundForAssignment, Enqueued False",
			conditions:  []*apipb.Condition{noTarget(), condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_FALSE, constants.KueueQueueNotFound)},
			wantChanged: true,
		},
		{name: "both already recorded", conditions: []*apipb.Condition{noTarget(), enqueued()}},
		{
			name:       "never over a recorded decision",
			conditions: []*apipb.Condition{condition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE, constants.ClusterMatched)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := testRayCluster(nil, "")
			rc.Generation = 2
			rc.Status.StatusConditions = tt.conditions
			before := rc.DeepCopy()

			changed := recordNoTargetResult(rc)
			assert.Equal(t, tt.wantChanged, changed)
			if !changed {
				assert.Equal(t, before, rc)
				return
			}
			scheduled := findCondition(rc.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, scheduled)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, scheduled.Status)
			assert.Equal(t, constants.NoClustersFoundForAssignment, scheduled.Reason)
			got := findCondition(rc.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, got)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, got.Reason)
		})
	}

	t.Run("unsupported type", func(t *testing.T) {
		assert.False(t, recordNoTargetResult(&v2pb.RayJob{}))
	})
}

func TestEnqueueAlreadyScheduledIsNoop(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "kueue-a")
	rc.Status.StatusConditions = []*apipb.Condition{{
		Type:    constants.ScheduledCondition,
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  constants.ClusterMatched,
		Message: "cluster kueue-a",
	}}

	// No expectations: an already scheduled job makes no decision.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Empty(t, handler.statusWrites())
	assertScheduledTo(t, &readBack().Status, "kueue-a")
}

func TestEnqueueStaleReadNeverOverwritesNewerDecision(t *testing.T) {
	g := gomock.NewController(t)
	// Stored: another decision already placed the job on cluster-b.
	rc := testRayCluster(projectLabels(), "cluster-b")
	rc.Status.StatusConditions = []*apipb.Condition{{
		Type:    constants.ScheduledCondition,
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  constants.ClusterMatched,
		Message: "cluster cluster-b",
	}}

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	// The decision's first read is stale: it predates the stored decision.
	handler.stale = testRayCluster(projectLabels(), "")

	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assertScheduledTo(t, &readBack().Status, "cluster-b")
}

func TestEnqueueSerializesDecisions(t *testing.T) {
	const jobs = 8
	g := gomock.NewController(t)

	objs := make([]client.Object, 0, jobs)
	for i := 0; i < jobs; i++ {
		rc := testRayCluster(nil, "")
		rc.Name = fmt.Sprintf("job-%d", i)
		objs = append(objs, rc)
	}

	probe := &serialProbe{}
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a")).AnyTimes()
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler := newTestQueue(t, probe, clusters, queues, kueueConfig(), objs...)
	probe.handler = handler

	start := make(chan struct{})
	errs := make(chan error, jobs)
	var wg sync.WaitGroup
	for _, obj := range objs {
		wg.Add(1)
		go func(rc *v2pb.RayCluster) {
			defer wg.Done()
			<-start
			errs <- q.Enqueue(context.Background(), schedulableJob(rc))
		}(obj.(*v2pb.RayCluster))
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.False(t, probe.interleaved, "a decision started before an earlier one was recorded")
	assert.Equal(t, jobs, probe.selects)
	assert.Len(t, handler.statusWrites(), jobs)
	for _, obj := range objs {
		assertScheduledTo(t, &readRayCluster(t, handler, obj.GetNamespace(), obj.GetName()).Status, "cluster-a")
	}
}

// serialProbe is an AssignmentStrategy that flags any decision starting
// before every earlier decision has been recorded.
type serialProbe struct {
	handler *countingHandler

	mu          sync.Mutex
	selects     int
	interleaved bool
}

func (p *serialProbe) Select(_ context.Context, _ framework.BatchJob) (*v2pb.AssignmentInfo, bool, string, error) {
	p.mu.Lock()
	if len(p.handler.statusWrites()) != p.selects {
		p.interleaved = true
	}
	p.selects++
	p.mu.Unlock()
	// Widen the window in which unserialized decisions would overlap.
	time.Sleep(2 * time.Millisecond)
	return &v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil
}

func TestEnqueueUsesExistingAssignmentWithoutStrategy(t *testing.T) {
	g := gomock.NewController(t)
	// A recorded assignment that was never marked Scheduled (legacy object)
	// is re-validated and then recorded as the decision.
	rc := testRayCluster(projectLabels(), "kueue-a")

	// No Select expectation: the existing assignment must be used.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)

	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(true, nil)

	q, handler, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	require.Len(t, handler.statusWrites(), 1)
	assertScheduledTo(t, &readBack().Status, "kueue-a")
}

func TestEnqueueExistingAssignmentToUnregisteredClusterSelectsAgain(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "gone")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("gone").Return(nil)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))
	queues := kueuemocks.NewMockLocalQueues(g)

	q, _, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assertScheduledTo(t, &readBack().Status, "cluster-a")
}

// replacedRayCluster is a RayCluster re-placed off the excluded clusters, so
// it has no assignment and its Launched condition records them.
func replacedRayCluster(t *testing.T, labels map[string]string, excluded ...string) *v2pb.RayCluster {
	t.Helper()
	rc := testRayCluster(labels, "")
	metadata, err := utils.DispatchState{Replacements: len(excluded), ExcludedClusters: excluded}.Metadata()
	require.NoError(t, err)
	rc.Status.StatusConditions = []*apipb.Condition{{
		Type: constants.LaunchedCondition, Status: apipb.CONDITION_STATUS_UNKNOWN, Metadata: metadata,
	}}
	return rc
}

// readyClusters is a cluster cache of the named clusters, all registered and
// Ready, in order.
func readyClusters(g *gomock.Controller, names ...string) *clustermocks.MockRegisteredClustersCache {
	byName := map[string]*v2pb.Cluster{}
	var ready []*v2pb.Cluster
	for _, name := range names {
		byName[name] = defaultCluster(name)
		ready = append(ready, byName[name])
	}
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetClusters(cluster.ReadyClusters).Return(ready).AnyTimes()
	clusters.EXPECT().GetCluster(gomock.Any()).DoAndReturn(func(name string) *v2pb.Cluster { return byName[name] }).AnyTimes()
	return clusters
}

func TestEnqueueReplacementExcludesEarlierTargets(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		excluded []string
		want     string
	}{
		{
			name:     "an excluded Ready cluster is skipped",
			excluded: []string{"cluster-a"},
			want:     "cluster-b",
		},
		{
			name:     "with every Ready cluster excluded, the decision is made without the exclusion",
			excluded: []string{"cluster-b", "cluster-a"},
			want:     "cluster-a",
		},
		{
			name:     "a pin is kept",
			labels:   map[string]string{constants.ClusterAffinityLabelKey: "cluster-a"},
			excluded: []string{"cluster-a"},
			want:     "cluster-a",
		},
		{
			name: "a job without exclusions is unchanged",
			want: "cluster-a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			clusters := readyClusters(g, "cluster-a", "cluster-b")
			strategy := framework.NewClusterOnlyAssignmentStrategy(clusters, logr.Discard())
			rc := replacedRayCluster(t, tt.labels, tt.excluded...)

			q, handler, readBack := setup(t, rc, strategy, clusters, kueuemocks.NewMockLocalQueues(g), kueueConfig())
			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))

			require.Len(t, handler.statusWrites(), 1)
			assertScheduledTo(t, &readBack().Status, tt.want)
		})
	}
}

func TestEnqueueReplacementWithAStrategyThatCannotExcludeSelects(t *testing.T) {
	g := gomock.NewController(t)
	rc := replacedRayCluster(t, nil, "cluster-a")

	// The mock implements only AssignmentStrategy.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))

	q, _, readBack := setup(t, rc, strategy, clusters, kueuemocks.NewMockLocalQueues(g), kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assertScheduledTo(t, &readBack().Status, "cluster-a")
}

func TestEnqueueQueueResolutionUsesOverrides(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "kueue-a")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "custom-queue").Return(true, nil)

	cfg := maconfig.SchedulerConfig{
		Backend: "kueue",
		Kueue: maconfig.KueueConfig{
			LocalQueueOverrides: map[string]string{"proj-ns": "custom-queue"},
		},
	}
	q, _, readBack := setup(t, rc, strategy, clusters, queues, cfg)
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assertScheduledTo(t, &readBack().Status, "kueue-a")
}

func TestEnqueueQueueNotFound(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(false, nil)

	q, _, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	got := readBack()
	cond := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, cond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
	assert.Equal(t, constants.KueueQueueNotFound, cond.Reason)
	assert.Contains(t, cond.Message, "ma-proj-ns")
	assert.Empty(t, got.Status.Assignment.GetCluster())
	assertNotScheduled(t, &got.Status)
}

func TestEnqueueQueueCheckFailed(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").
		Return(false, errors.New("cluster unreachable"))

	q, _, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)

	got := readBack()
	cond := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, cond)
	assert.Equal(t, constants.KueueQueueCheckFailed, cond.Reason)
	assert.Empty(t, got.Status.Assignment.GetCluster())
	assertNotScheduled(t, &got.Status)
}

func TestEnqueueMissingProjectLabelFallsBackToNamespace(t *testing.T) {
	g := gomock.NewController(t)
	// No ma/project-name label: the job's namespace ("proj-ns") is the
	// project identity, so the queue resolves to ma-proj-ns.
	rc := testRayCluster(map[string]string{"unrelated": "x"}, "kueue-a")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(true, nil)

	q, _, readBack := setup(t, rc, strategy, clusters, queues, kueueConfig())
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assertScheduledTo(t, &readBack().Status, "kueue-a")
}

func TestEnqueueQueueIdentity(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]string
		trustLabel bool
		wantQueue  string
	}{
		{name: "label set, untrusted: namespace", labels: projectLabels(), trustLabel: false, wantQueue: "ma-proj-ns"},
		{name: "label set, trusted: label", labels: projectLabels(), trustLabel: true, wantQueue: "ma-proj1"},
		{name: "label unset, untrusted: namespace", labels: nil, trustLabel: false, wantQueue: "ma-proj-ns"},
		{name: "label unset, trusted: namespace", labels: nil, trustLabel: true, wantQueue: "ma-proj-ns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomock.NewController(t)
			rc := testRayCluster(tt.labels, "kueue-a")

			strategy := frameworkmocks.NewMockAssignmentEngine(g)
			target := kueueCluster("kueue-a")
			clusters := clustermocks.NewMockRegisteredClustersCache(g)
			clusters.EXPECT().GetCluster("kueue-a").Return(target)

			queues := kueuemocks.NewMockLocalQueues(g)
			queues.EXPECT().Exists(gomock.Any(), target, "default", tt.wantQueue).Return(true, nil)

			cfg := maconfig.SchedulerConfig{
				Backend: "kueue",
				Kueue:   maconfig.KueueConfig{TrustProjectLabel: tt.trustLabel},
			}
			q, _, readBack := setup(t, rc, strategy, clusters, queues, cfg)
			require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
			assertScheduledTo(t, &readBack().Status, "kueue-a")
		})
	}
}

func TestEnqueueStrategyErrorPropagates(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(nil, false, "", fmt.Errorf("strategy exploded"))

	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler, _ := setup(t, rc, strategy, clusters, queues, kueueConfig())
	// A strategy error is not "no target": it surfaces, and nothing is
	// written, so the caller retries with a fresh decision.
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strategy exploded")
	assert.Empty(t, handler.statusWrites())
}

func TestEnqueueRecordsSparkJob(t *testing.T) {
	g := gomock.NewController(t)
	sj := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-1", Namespace: "proj-ns"}}

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))
	queues := kueuemocks.NewMockLocalQueues(g)

	q, handler := newTestQueue(t, strategy, clusters, queues, kueueConfig(), sj)
	require.NoError(t, q.Enqueue(context.Background(), framework.BatchSparkJob{SparkJob: sj}))

	got := &v2pb.SparkJob{}
	require.NoError(t, handler.Handler.Get(context.Background(), sj.Namespace, sj.Name, &metav1.GetOptions{}, got))
	assert.Equal(t, "cluster-a", got.Status.Assignment.GetCluster())
	cond := findCondition(got.Status.StatusConditions, constants.ScheduledCondition)
	require.NotNil(t, cond)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, cond.Status)
	assert.Equal(t, "cluster cluster-a", cond.Message)
}

func TestRecordPlacement(t *testing.T) {
	condition := func(condType string, status apipb.ConditionStatus, reason string) *apipb.Condition {
		return &apipb.Condition{Type: condType, Status: status, Reason: reason, LastUpdatedTimestamp: 7}
	}
	tests := []struct {
		name         string
		conditions   []*apipb.Condition
		wantEnqueued *apipb.Condition // nil: set True/AddedToSchedulerQueue by the record
	}{
		{name: "no conditions"},
		{
			name:         "Enqueued already True is kept",
			conditions:   []*apipb.Condition{condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, constants.AddedToSchedulerQueue)},
			wantEnqueued: condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, constants.AddedToSchedulerQueue),
		},
		{
			name:       "Enqueued False from a failed validation becomes True",
			conditions: []*apipb.Condition{condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_FALSE, constants.KueueQueueNotFound)},
		},
		{
			name:       "Scheduled False with no target becomes True",
			conditions: []*apipb.Condition{condition(constants.ScheduledCondition, apipb.CONDITION_STATUS_FALSE, constants.NoClustersFoundForAssignment)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := testRayCluster(nil, "")
			rc.Generation = 3
			rc.Status.StatusConditions = tt.conditions

			require.True(t, recordPlacement(rc, "cluster-x", tierNone))
			assertScheduledTo(t, &rc.Status, "cluster-x")
			scheduled := findCondition(rc.Status.StatusConditions, constants.ScheduledCondition)
			assert.Equal(t, int64(3), scheduled.ObservedGeneration)

			enqueued := findCondition(rc.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued)
			if tt.wantEnqueued != nil {
				assert.Equal(t, tt.wantEnqueued, enqueued)
				return
			}
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
			assert.Equal(t, int64(3), enqueued.ObservedGeneration)
		})
	}

	t.Run("already scheduled: no-op", func(t *testing.T) {
		rc := testRayCluster(nil, "cluster-b")
		rc.Status.StatusConditions = []*apipb.Condition{
			condition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE, constants.ClusterMatched),
			condition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_FALSE, constants.KueueQueueNotFound),
		}
		before := rc.DeepCopy()
		assert.False(t, recordPlacement(rc, "cluster-x", tierFitsNow))
		assert.Equal(t, before, rc)
	})

	t.Run("spark job", func(t *testing.T) {
		sj := &v2pb.SparkJob{}
		require.True(t, recordPlacement(sj, "cluster-x", tierNone))
		assert.Equal(t, "cluster-x", sj.Status.Assignment.GetCluster())
		assert.True(t, findCondition(sj.Status.StatusConditions, constants.ScheduledCondition).Status == apipb.CONDITION_STATUS_TRUE)
	})

	t.Run("unsupported type", func(t *testing.T) {
		assert.False(t, recordPlacement(&v2pb.RayJob{}, "cluster-x", tierNone))
	})

	t.Run("a ranked cluster's tier is appended to the message", func(t *testing.T) {
		for _, tier := range []Tier{tierFitsNow, tierQueueDepth, tierUnsynced, tierHeld} {
			rc := testRayCluster(nil, "")
			require.True(t, recordPlacement(rc, "cluster-x", tier))
			assert.Equal(t, "cluster-x", rc.Status.Assignment.GetCluster())
			scheduled := findCondition(rc.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, scheduled)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, scheduled.Status)
			assert.Equal(t, constants.ClusterMatched, scheduled.Reason)
			assert.Equal(t, fmt.Sprintf("cluster cluster-x, tier %d", tier), scheduled.Message)
		}
	})
}

func TestRecordNoPlacement(t *testing.T) {
	tests := []struct {
		name        string
		scheduled   *apipb.Condition // nil: no Scheduled condition
		wantChanged bool
	}{
		{name: "no Scheduled condition", wantChanged: true},
		{name: "Scheduled Unknown", scheduled: &apipb.Condition{Status: apipb.CONDITION_STATUS_UNKNOWN}, wantChanged: true},
		{
			name:        "Scheduled False for another reason",
			scheduled:   &apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoClusterMatchedRequirements"},
			wantChanged: true,
		},
		{
			name:        "already False/NoClustersFoundForAssignment",
			scheduled:   &apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Reason: constants.NoClustersFoundForAssignment},
			wantChanged: false,
		},
		{
			name:        "never over a recorded decision",
			scheduled:   &apipb.Condition{Status: apipb.CONDITION_STATUS_TRUE, Reason: constants.ClusterMatched},
			wantChanged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := testRayCluster(nil, "")
			if tt.scheduled != nil {
				tt.scheduled.Type = constants.ScheduledCondition
				rc.Status.StatusConditions = []*apipb.Condition{tt.scheduled}
			}
			before := rc.DeepCopy()

			changed := recordNoPlacement(rc)
			assert.Equal(t, tt.wantChanged, changed)
			if !changed {
				assert.Equal(t, before, rc)
				return
			}
			cond := findCondition(rc.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, cond)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
			assert.Equal(t, constants.NoClustersFoundForAssignment, cond.Reason)
		})
	}
}

func findCondition(conds []*apipb.Condition, condType string) *apipb.Condition {
	for _, c := range conds {
		if c.GetType() == condType {
			return c
		}
	}
	return nil
}
