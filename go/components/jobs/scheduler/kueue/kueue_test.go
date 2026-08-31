package kueue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
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

func kueueCluster(name string) *v2pb.Cluster {
	return &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
	}
}

func defaultCluster(name string) *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func setup(t *testing.T, rc *v2pb.RayCluster, strategy framework.AssignmentStrategy,
	clusters *clustermocks.MockRegisteredClustersCache, queues LocalQueues, cfg maconfig.SchedulerConfig,
) (*KueueJobQueue, *recordingDelegate, func() *v2pb.RayCluster) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(rc).
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

	readBack := func() *v2pb.RayCluster {
		got := &v2pb.RayCluster{}
		require.NoError(t, handler.Get(context.Background(), rc.Namespace, rc.Name, &metav1.GetOptions{}, got))
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
