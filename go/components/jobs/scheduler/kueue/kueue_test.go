package kueue

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/kueuemocks"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type recordingDelegate struct {
	calls int
	err   error
}

func (d *recordingDelegate) Enqueue(_ context.Context, _ matypes.SchedulableJob) error {
	d.calls++
	return d.err
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

func testSparkJob(labels map[string]string, assignedCluster string) *v2pb.SparkJob {
	sj := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "spark-1",
			Namespace: "proj-ns",
			Labels:    labels,
		},
	}
	if assignedCluster != "" {
		sj.Status.Assignment = &v2pb.AssignmentInfo{Cluster: assignedCluster}
	}
	return sj
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

// newQueue builds a KueueJobQueue over a fake API server that already holds obj.
// The status subresource is enabled for both job kinds, so a condition the
// decorator writes can be read back: read fetches the stored obj into its
// argument.
func newQueue(t *testing.T, obj client.Object, strategy framework.AssignmentStrategy,
	clusters *clustermocks.MockRegisteredClustersCache, queues LocalQueues, cfg maconfig.SchedulerConfig,
) (*KueueJobQueue, *recordingDelegate, func(into client.Object)) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(obj).
		WithStatusSubresource(&v2pb.RayCluster{}, &v2pb.SparkJob{}).
		Build()
	handler := apiHandler.NewFakeAPIHandler(fakeClient)

	delegate := &recordingDelegate{}
	q := NewKueueJobQueue(Params{
		Delegate:           delegate,
		Handler:            handler,
		AssignmentStrategy: strategy,
		ClusterCache:       clusters,
		LocalQueues:        queues,
		Config:             cfg,
		Scope:              tally.NewTestScope("test", nil),
		Logger:             logr.Discard(),
	})

	read := func(into client.Object) {
		require.NoError(t, handler.Get(context.Background(), obj.GetNamespace(), obj.GetName(), &metav1.GetOptions{}, into))
	}
	return q, delegate, read
}

func setup(t *testing.T, rc *v2pb.RayCluster, strategy framework.AssignmentStrategy,
	clusters *clustermocks.MockRegisteredClustersCache, queues LocalQueues, cfg maconfig.SchedulerConfig,
) (*KueueJobQueue, *recordingDelegate, func() *v2pb.RayCluster) {
	q, delegate, read := newQueue(t, rc, strategy, clusters, queues, cfg)
	readBack := func() *v2pb.RayCluster {
		got := &v2pb.RayCluster{}
		read(got)
		return got
	}
	return q, delegate, readBack
}

func schedulableJob(rc *v2pb.RayCluster) matypes.SchedulableJob {
	return framework.BatchRayCluster{RayCluster: rc}
}

func projectLabels() map[string]string {
	return map[string]string{constants.ProjectNameLabelKey: "proj1"}
}

func TestEnqueuePassThroughForNonKueueCluster(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "cluster-a"}, true, "", nil)

	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("cluster-a").Return(defaultCluster("cluster-a"))

	queues := kueuemocks.NewMockLocalQueues(g) // no Exists expectation: must not be called

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
}

func TestEnqueuePassThroughWhenNoTargetPredicted(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(nil, false, "no clusters", nil)

	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
}

func TestEnqueueValidatesAndDelegatesWhenQueueExists(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)

	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj1").Return(true, nil)

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
}

func TestEnqueueUsesExistingAssignmentWithoutStrategy(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "kueue-a")

	// No Select expectation: the existing assignment must be used.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)

	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj1").Return(true, nil)

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
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
			LocalQueueOverrides: map[string]string{"proj1": "custom-queue"},
		},
	}
	q, delegate, _ := setup(t, rc, strategy, clusters, queues, cfg)
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
}

func TestEnqueueQueueNotFound(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "kueue-a")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj1").Return(false, nil)

	q, delegate, readBack := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Equal(t, 0, delegate.calls)

	got := readBack()
	cond := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, cond)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
	assert.Equal(t, constants.KueueQueueNotFound, cond.Reason)
	assert.Contains(t, cond.Message, "ma-proj1")
}

func TestEnqueueQueueCheckFailed(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "kueue-a")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj1").
		Return(false, errors.New("cluster unreachable"))

	q, delegate, readBack := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)
	assert.Equal(t, 0, delegate.calls)

	cond := findCondition(readBack().Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, cond)
	assert.Equal(t, constants.KueueQueueCheckFailed, cond.Reason)
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

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	require.NoError(t, q.Enqueue(context.Background(), schedulableJob(rc)))
	assert.Equal(t, 1, delegate.calls)
}

func TestEnqueueDelegateErrorPropagates(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	strategy.EXPECT().Select(gomock.Any(), gomock.Any()).
		Return(nil, false, "", fmt.Errorf("strategy exploded"))

	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	delegate.err = errors.New("queue full")
	// A strategy error means no prediction; the job passes through and the
	// delegate's own error surfaces exactly as it does today.
	err := q.Enqueue(context.Background(), schedulableJob(rc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queue full")
	assert.Equal(t, 1, delegate.calls)
}

func findCondition(conds []*apipb.Condition, condType string) *apipb.Condition {
	for _, c := range conds {
		if c.GetType() == condType {
			return c
		}
	}
	return nil
}

// TestEnqueueSparkJobValidatesAgainstLocalQueue: a Kueue-managed Spark
// job clears the same pre-flight a RayCluster does. fetchLatestJob finds the
// SparkJob, its LocalQueue is resolved from the project label, and the job
// reaches the delegate behind it.
//
// The lookup is expected in k8sengine.SparkLocalNamespace, the namespace Spark
// is projected into. Enqueue names RayLocalNamespace, so while the two constants
// are equal this test cannot tell them apart. If they ever differ, the
// compile-time assertion at the top of kueue.go fails the build before
// this test can run, or, if either has become a var,
// TestRayAndSparkShareLocalNamespace fails.
func TestEnqueueSparkJobValidatesAgainstLocalQueue(t *testing.T) {
	g := gomock.NewController(t)
	sj := testSparkJob(projectLabels(), "kueue-a")

	// No Select expectation: the existing assignment must be used.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)

	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, k8sengine.SparkLocalNamespace, "ma-proj1").Return(true, nil)

	q, delegate, _ := newQueue(t, sj, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})

	require.NoError(t, q.Enqueue(context.Background(), framework.BatchSparkJob{SparkJob: sj}))
	assert.Equal(t, 1, delegate.calls)
}

// TestRayAndSparkShareLocalNamespace: Enqueue looks every job's LocalQueue up
// in RayLocalNamespace, which is right for a Spark job only while the two
// namespaces are equal. The compile-time assertion in kueue.go guards that only
// while both are constants; this test keeps guarding it if either becomes a
// var.
func TestRayAndSparkShareLocalNamespace(t *testing.T) {
	require.Equal(t, k8sengine.RayLocalNamespace, k8sengine.SparkLocalNamespace,
		"Enqueue looks a Spark job's LocalQueue up in RayLocalNamespace")
}

// jobKind is one kind of job the Kueue decorator gates, so a test can run once
// per kind. newJob returns the object to store and the SchedulableJob the
// controller would hand to Enqueue, already assigned to "kueue-a". conditions
// reads the stored object's status conditions back through read.
type jobKind struct {
	name           string
	localNamespace string
	newJob         func(labels map[string]string, namespace string) (client.Object, matypes.SchedulableJob)
	conditions     func(read func(client.Object)) []*apipb.Condition
}

func jobKinds() []jobKind {
	return []jobKind{
		{
			name:           "RayCluster",
			localNamespace: k8sengine.RayLocalNamespace,
			newJob: func(labels map[string]string, namespace string) (client.Object, matypes.SchedulableJob) {
				rc := testRayCluster(labels, "kueue-a")
				rc.Namespace = namespace
				return rc, framework.BatchRayCluster{RayCluster: rc}
			},
			conditions: func(read func(client.Object)) []*apipb.Condition {
				got := &v2pb.RayCluster{}
				read(got)
				return got.Status.StatusConditions
			},
		},
		{
			name:           "SparkJob",
			localNamespace: k8sengine.SparkLocalNamespace,
			newJob: func(labels map[string]string, namespace string) (client.Object, matypes.SchedulableJob) {
				sj := testSparkJob(labels, "kueue-a")
				sj.Namespace = namespace
				return sj, framework.BatchSparkJob{SparkJob: sj}
			},
			conditions: func(read func(client.Object)) []*apipb.Condition {
				got := &v2pb.SparkJob{}
				read(got)
				return got.Status.StatusConditions
			},
		},
	}
}

// TestEnqueueFailurePersistsEnqueuedCondition: each way the pre-flight can
// refuse a job headed for a Kueue-managed cluster, for both kinds of job the
// decorator gates, leaves Enqueued=False on the stored object with the reason
// and the identifiers an operator needs, returns an error, and never reaches the
// delegate.
func TestEnqueueFailurePersistsEnqueuedCondition(t *testing.T) {
	errUnreachable := errors.New("cluster unreachable")

	failures := []struct {
		name string
		// labels and namespace decide the project identity; a job with neither has
		// none.
		labels    map[string]string
		namespace string
		// lookup arms the LocalQueues mock. nil leaves it unarmed, so any lookup
		// fails the test.
		lookup      func(queues *kueuemocks.MockLocalQueues, target *v2pb.Cluster, namespace string)
		wantReason  string
		wantErr     string
		wantErrIs   error
		wantMessage []string
	}{
		{
			name:      "LocalQueue not found",
			labels:    projectLabels(),
			namespace: "proj-ns",
			lookup: func(queues *kueuemocks.MockLocalQueues, target *v2pb.Cluster, namespace string) {
				queues.EXPECT().Exists(gomock.Any(), target, namespace, "ma-proj1").Return(false, nil)
			},
			wantReason:  constants.KueueQueueNotFound,
			wantErr:     "not found",
			wantMessage: []string{"ma-proj1", "kueue-a"},
		},
		{
			name:      "LocalQueue check failed",
			labels:    projectLabels(),
			namespace: "proj-ns",
			lookup: func(queues *kueuemocks.MockLocalQueues, target *v2pb.Cluster, namespace string) {
				queues.EXPECT().Exists(gomock.Any(), target, namespace, "ma-proj1").Return(false, errUnreachable)
			},
			wantReason:  constants.KueueQueueCheckFailed,
			wantErrIs:   errUnreachable,
			wantMessage: []string{"ma-proj1", "kueue-a", errUnreachable.Error()},
		},
		{
			name: "no project",
			// Neither a ma/project-name label nor a namespace to fall back on.
			lookup:      nil,
			wantReason:  constants.KueueQueueNotFound,
			wantErr:     "cannot resolve a project",
			wantMessage: []string{"kueue-a", constants.ProjectNameLabelKey},
		},
	}

	for _, kind := range jobKinds() {
		for _, tc := range failures {
			t.Run(kind.name+"/"+tc.name, func(t *testing.T) {
				g := gomock.NewController(t)
				obj, job := kind.newJob(tc.labels, tc.namespace)

				// No Select expectation: the existing assignment is used.
				strategy := frameworkmocks.NewMockAssignmentEngine(g)
				target := kueueCluster("kueue-a")
				clusters := clustermocks.NewMockRegisteredClustersCache(g)
				clusters.EXPECT().GetCluster("kueue-a").Return(target)

				queues := kueuemocks.NewMockLocalQueues(g)
				if tc.lookup != nil {
					tc.lookup(queues, target, kind.localNamespace)
				}

				q, delegate, read := newQueue(t, obj, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
				err := q.Enqueue(context.Background(), job)
				require.Error(t, err)
				if tc.wantErr != "" {
					assert.Contains(t, err.Error(), tc.wantErr)
				}
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
				assert.Equal(t, 0, delegate.calls)

				cond := findCondition(kind.conditions(read), constants.EnqueuedCondition)
				require.NotNil(t, cond, "the Enqueued condition was not persisted")
				assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
				assert.Equal(t, tc.wantReason, cond.Reason)
				for _, want := range tc.wantMessage {
					assert.Contains(t, cond.Message, want)
				}
			})
		}
	}
}

// stallUntilDone is an Exists for a compute cluster that has stopped answering:
// the LocalQueue GET blocks until its context is done, then fails with the
// context's error, as client-go's does.
func stallUntilDone(ctx context.Context, _ *v2pb.Cluster, _, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

// conditionWrite is an Enqueued condition the decorator sent to the API server,
// with how long the write's context had left when it was sent.
type conditionWrite struct {
	status      apipb.ConditionStatus
	reason      string
	timeLeft    time.Duration
	hasDeadline bool
}

// liveContextHandler wraps the fake API handler, which ignores its context,
// with what client-go does on a context that is already done: the call fails
// with the context's error and sends nothing. UpdateStatusWithRetries reads the
// job before it writes, so a write on an expired context fails at that Get, as
// it does in production. UpdateStatus fails the test outright if its context
// is done, and records each Enqueued condition it sends.
type liveContextHandler struct {
	api.Handler
	t      *testing.T
	writes []conditionWrite
}

// wrapHandler puts a liveContextHandler between q and its API handler.
func wrapHandler(t *testing.T, q *KueueJobQueue) *liveContextHandler {
	h := &liveContextHandler{Handler: q.handler, t: t}
	q.handler = h
	return h
}

func (h *liveContextHandler) Get(ctx context.Context, namespace, name string, opts *metav1.GetOptions, obj client.Object) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.Handler.Get(ctx, namespace, name, opts, obj)
}

func (h *liveContextHandler) UpdateStatus(ctx context.Context, obj client.Object, opts *metav1.UpdateOptions) error {
	if !assert.NoError(h.t, ctx.Err(), "a status write was issued on a context that is already done") {
		return ctx.Err()
	}
	var conds []*apipb.Condition
	switch o := obj.(type) {
	case *v2pb.RayCluster:
		conds = o.Status.StatusConditions
	case *v2pb.SparkJob:
		conds = o.Status.StatusConditions
	}
	if cond := findCondition(conds, constants.EnqueuedCondition); cond != nil {
		deadline, ok := ctx.Deadline()
		h.writes = append(h.writes, conditionWrite{
			status:      cond.Status,
			reason:      cond.Reason,
			timeLeft:    time.Until(deadline),
			hasDeadline: ok,
		})
	}
	return h.Handler.UpdateStatus(ctx, obj, opts)
}

// TestEnqueueRecordsCheckFailureAfterValidationDeadline: a compute cluster that
// has stopped answering holds the LocalQueue check until the validation
// deadline runs out. The rejection is still recorded, for both kinds of job the
// decorator gates, because the condition write gets a deadline of its own from
// the caller's context; on the expired validation context it could not be sent.
func TestEnqueueRecordsCheckFailureAfterValidationDeadline(t *testing.T) {
	for _, kind := range jobKinds() {
		t.Run(kind.name, func(t *testing.T) {
			g := gomock.NewController(t)
			obj, job := kind.newJob(projectLabels(), "proj-ns")

			// No Select expectation: the existing assignment is used.
			strategy := frameworkmocks.NewMockAssignmentEngine(g)
			target := kueueCluster("kueue-a")
			clusters := clustermocks.NewMockRegisteredClustersCache(g)
			clusters.EXPECT().GetCluster("kueue-a").Return(target)

			queues := kueuemocks.NewMockLocalQueues(g)
			queues.EXPECT().Exists(gomock.Any(), target, kind.localNamespace, "ma-proj1").DoAndReturn(stallUntilDone)

			q, delegate, read := newQueue(t, obj, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
			handler := wrapHandler(t, q)
			// The validation deadline runs out before the check's cap does.
			q.validateTimeout = 50 * time.Millisecond
			q.queueCheckTimeout = time.Minute

			err := q.Enqueue(context.Background(), job)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Equal(t, 0, delegate.calls)

			require.Len(t, handler.writes, 1, "want exactly one Enqueued condition write")
			write := handler.writes[0]
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, write.status)
			assert.Equal(t, constants.KueueQueueCheckFailed, write.reason)
			require.True(t, write.hasDeadline, "the condition write ran without a deadline")
			assert.LessOrEqual(t, write.timeLeft, q.conditionWriteTimeout)

			cond := findCondition(kind.conditions(read), constants.EnqueuedCondition)
			require.NotNil(t, cond, "the Enqueued condition was not persisted")
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, cond.Status)
			assert.Equal(t, constants.KueueQueueCheckFailed, cond.Reason)
		})
	}
}

// TestEnqueueCapsLocalQueueCheck: the LocalQueue check runs under a cap shorter
// than the validation deadline, so a compute cluster that has stopped answering
// holds the caller for at most the cap, and the rejection is recorded.
func TestEnqueueCapsLocalQueueCheck(t *testing.T) {
	g := gomock.NewController(t)
	sj := testSparkJob(projectLabels(), "kueue-a")

	// No Select expectation: the existing assignment is used.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	var calledAt, deadline time.Time
	var hasDeadline bool
	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, k8sengine.SparkLocalNamespace, "ma-proj1").DoAndReturn(
		func(ctx context.Context, cluster *v2pb.Cluster, namespace, name string) (bool, error) {
			calledAt = time.Now()
			deadline, hasDeadline = ctx.Deadline()
			return stallUntilDone(ctx, cluster, namespace, name)
		})

	q, delegate, read := newQueue(t, sj, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	// The check's cap runs out long before the validation deadline does.
	q.validateTimeout = 5 * time.Second
	q.queueCheckTimeout = 50 * time.Millisecond

	err := q.Enqueue(context.Background(), framework.BatchSparkJob{SparkJob: sj})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 0, delegate.calls)

	require.True(t, hasDeadline, "the LocalQueue check ran without a deadline")
	assert.LessOrEqual(t, deadline.Sub(calledAt), q.queueCheckTimeout,
		"the LocalQueue check may run past its cap")

	got := &v2pb.SparkJob{}
	read(got)
	cond := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
	require.NotNil(t, cond, "the Enqueued condition was not persisted")
	assert.Equal(t, constants.KueueQueueCheckFailed, cond.Reason)
}

// TestEnqueueCancelledCallerCancelsConditionWrite: the condition write's
// deadline is derived from the caller's context, so a caller that is cancelled
// while the check is outstanding (the manager shutting down) cancels the write
// as well, rather than leaving it to run on a deadline of its own.
func TestEnqueueCancelledCallerCancelsConditionWrite(t *testing.T) {
	g := gomock.NewController(t)
	sj := testSparkJob(projectLabels(), "kueue-a")

	// No Select expectation: the existing assignment is used.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	target := kueueCluster("kueue-a")
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	clusters.EXPECT().GetCluster("kueue-a").Return(target)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queues := kueuemocks.NewMockLocalQueues(g)
	queues.EXPECT().Exists(gomock.Any(), target, k8sengine.SparkLocalNamespace, "ma-proj1").DoAndReturn(
		func(checkCtx context.Context, cluster *v2pb.Cluster, namespace, name string) (bool, error) {
			cancel()
			return stallUntilDone(checkCtx, cluster, namespace, name)
		})

	q, delegate, _ := newQueue(t, sj, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	handler := wrapHandler(t, q)

	err := q.Enqueue(ctx, framework.BatchSparkJob{SparkJob: sj})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, delegate.calls)
	assert.Empty(t, handler.writes, "the Enqueued condition was written for a caller that had been cancelled")
}

// TestEnqueueUnrecognizedJobType: a job type the decorator has no arm for is
// refused before any cluster is predicted or queue looked up, and the error says
// which type it was.
func TestEnqueueUnrecognizedJobType(t *testing.T) {
	g := gomock.NewController(t)
	rc := testRayCluster(projectLabels(), "")

	// Strict mocks with no expectations: nothing past fetchLatestJob may run.
	strategy := frameworkmocks.NewMockAssignmentEngine(g)
	clusters := clustermocks.NewMockRegisteredClustersCache(g)
	queues := kueuemocks.NewMockLocalQueues(g)

	q, delegate, _ := setup(t, rc, strategy, clusters, queues, maconfig.SchedulerConfig{Backend: "kueue"})
	err := q.Enqueue(context.Background(), matypes.NewSchedulableJob(matypes.SchedulableJobParams{
		Name:       rc.Name,
		Namespace:  rc.Namespace,
		Generation: 1,
		JobType:    matypes.JobType(99),
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized job type 99")
	assert.Equal(t, 0, delegate.calls)
}
