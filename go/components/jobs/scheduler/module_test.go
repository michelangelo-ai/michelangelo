package scheduler

import (
	"context"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sched "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/kueuemocks"
	kueuestate "github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state/statemocks"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func setupProvideIn(t *testing.T, backend string, objs ...client.Object) ProvideIn {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	mockClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v2pb.RayCluster{}).
		Build()

	mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
		Scheme: scheme,
		Logger: zapr.NewLogger(zaptest.NewLogger(t)),
	})
	require.NoError(t, err)

	g := gomock.NewController(t)
	factory := handlermocks.NewMockFactory(g)
	factory.EXPECT().
		GetAPIHandler(gomock.Any()).
		Return(apiHandler.NewFakeAPIHandler(mockClient), nil).
		AnyTimes()
	strategy := frameworkmocks.NewMockAssignmentEngine(g)

	scheduler := NewScheduler(Params{
		Manager:            mgr,
		Queue:              sched.New().Queue,
		ClusterCache:       setupMockClusterCache(g),
		Scope:              tally.NewTestScope("test", nil),
		APIHandlerFactory:  factory,
		AssignmentStrategy: strategy,
	})

	return ProvideIn{
		Scheduler:          scheduler,
		Config:             maconfig.SchedulerConfig{Backend: backend},
		Manager:            mgr,
		APIHandlerFactory:  factory,
		AssignmentStrategy: strategy,
		ClusterCache:       setupMockClusterCache(g),
		LocalQueues:        kueuemocks.NewMockLocalQueues(g),
		KueueState:         statemocks.NewMockState(g),
		KueueDemand:        statemocks.NewMockDemand(g),
		Scope:              tally.NewTestScope("test", nil),
	}
}

func TestProvideDefaultBackend(t *testing.T) {
	in := setupProvideIn(t, "")
	q, err := provide(in)
	require.NoError(t, err)
	assert.Same(t, in.Scheduler, q)
}

func TestProvideExplicitDefaultBackend(t *testing.T) {
	in := setupProvideIn(t, "default")
	q, err := provide(in)
	require.NoError(t, err)
	assert.Same(t, in.Scheduler, q)
}

func TestProvideKueueBackend(t *testing.T) {
	rc := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc-1", Namespace: "proj-ns"}}
	in := setupProvideIn(t, "kueue", rc)
	q, err := provide(in)
	require.NoError(t, err)
	kq, ok := q.(*kueue.KueueJobQueue)
	require.True(t, ok, "expected *kueue.KueueJobQueue, got %T", q)

	// The Kueue backend makes the decision itself rather than delegating to
	// the default scheduler: the scheduler's queue stays empty and the
	// outcome is persisted on the job.
	in.AssignmentStrategy.(*frameworkmocks.MockAssignmentEngine).EXPECT().
		Select(gomock.Any(), gomock.Any()).
		Return(nil, false, "", nil)
	require.NoError(t, kq.Enqueue(context.Background(), framework.BatchRayCluster{RayCluster: rc}))
	assert.Equal(t, 0, in.Scheduler.internalQueue.Length())

	handler, err := in.APIHandlerFactory.GetAPIHandler(nil)
	require.NoError(t, err)
	got := &v2pb.RayCluster{}
	require.NoError(t, handler.Get(context.Background(), rc.Namespace, rc.Name, &metav1.GetOptions{}, got))
	require.Len(t, got.Status.StatusConditions, 2)
	scheduled := got.Status.StatusConditions[0]
	assert.Equal(t, constants.ScheduledCondition, scheduled.Type)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, scheduled.Status)
	assert.Equal(t, constants.NoClustersFoundForAssignment, scheduled.Reason)
	// The job waits in the queue for a target. Under this backend the queue
	// is the only writer of Enqueued.
	enqueued := got.Status.StatusConditions[1]
	assert.Equal(t, constants.EnqueuedCondition, enqueued.Type)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
	assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)
}

// TestProvideKueueBackendAssignmentStrategy places a RayCluster on a
// Kueue-managed cluster under each jobs.scheduler.assignmentStrategy:
// cluster_only, the default, decides with the assignment strategy and
// validates the LocalQueue as in Phase 0; kueue_aware decides by the Kueue
// state, which records the tier; kueue_aware_shadow assigns as cluster_only
// and consults the Kueue state beside it.
func TestProvideKueueBackendAssignmentStrategy(t *testing.T) {
	tests := []struct {
		strategy    string
		kueueAware  bool
		clusterOnly bool
		wantMessage string
	}{
		{strategy: "", clusterOnly: true, wantMessage: "cluster kueue-a"},
		{strategy: maconfig.AssignmentStrategyClusterOnly, clusterOnly: true, wantMessage: "cluster kueue-a"},
		{strategy: maconfig.AssignmentStrategyKueueAwareShadow, kueueAware: true, clusterOnly: true, wantMessage: "cluster kueue-a"},
		{strategy: maconfig.AssignmentStrategyKueueAware, kueueAware: true, wantMessage: "cluster kueue-a, tier 1"},
	}
	for _, tt := range tests {
		t.Run("strategy="+tt.strategy, func(t *testing.T) {
			rc := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc-1", Namespace: "proj-ns"}}
			in := setupProvideIn(t, "kueue", rc)
			in.Config.AssignmentStrategy = tt.strategy

			g := gomock.NewController(t)
			target := &v2pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "kueue-a"},
				Spec:       v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
			}
			clusters := clustermocks.NewMockRegisteredClustersCache(g)
			clusters.EXPECT().GetCluster("kueue-a").Return(target).AnyTimes()
			clusters.EXPECT().GetClusters(gomock.Any()).Return([]*v2pb.Cluster{target}).AnyTimes()
			in.ClusterCache = clusters
			queues := kueuemocks.NewMockLocalQueues(g)
			in.LocalQueues = queues

			// Calls not expected here fail the test: cluster_only consults no
			// Kueue state, and kueue_aware neither the assignment strategy
			// nor the LocalQueue directly.
			if tt.clusterOnly {
				in.AssignmentStrategy.(*frameworkmocks.MockAssignmentEngine).EXPECT().
					Select(gomock.Any(), gomock.Any()).
					Return(&v2pb.AssignmentInfo{Cluster: "kueue-a"}, true, "", nil)
				queues.EXPECT().Exists(gomock.Any(), target, "default", "ma-proj-ns").Return(true, nil)
			}
			if tt.kueueAware {
				cq := &kueuestate.ClusterQueueView{
					Name:             "cq-a",
					FlavorCount:      1,
					CoveredResources: []corev1.ResourceName{corev1.ResourceCPU},
					Nominal:          corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")},
				}
				in.KueueState.(*statemocks.MockState).EXPECT().Lookup("kueue-a", "ma-proj-ns").
					Return(kueuestate.View{Synced: true, Exists: true, Active: true, ClusterQueue: cq}, nil).MinTimes(1)
				demand := in.KueueDemand.(*statemocks.MockDemand)
				demand.EXPECT().Ask(gomock.Any(), target).
					Return(corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}, nil)
				demand.EXPECT().InFlight(gomock.Any(), target, gomock.Any()).Return(kueuestate.InFlight{}, nil)
				// the recorded assignment is in flight to its cluster
				demand.EXPECT().RecordAssignment(gomock.Any(), "kueue-a", gomock.Any())
			}

			q, err := provide(in)
			require.NoError(t, err)
			require.NoError(t, q.Enqueue(context.Background(), framework.BatchRayCluster{RayCluster: rc}))

			handler, err := in.APIHandlerFactory.GetAPIHandler(nil)
			require.NoError(t, err)
			got := &v2pb.RayCluster{}
			require.NoError(t, handler.Get(context.Background(), rc.Namespace, rc.Name, &metav1.GetOptions{}, got))
			assert.Equal(t, "kueue-a", got.Status.Assignment.GetCluster())
			var scheduled *apipb.Condition
			for _, cond := range got.Status.StatusConditions {
				if cond.Type == constants.ScheduledCondition {
					scheduled = cond
				}
			}
			require.NotNil(t, scheduled)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, scheduled.Status)
			assert.Equal(t, tt.wantMessage, scheduled.Message)
		})
	}
}

func TestProvideUnknownBackend(t *testing.T) {
	in := setupProvideIn(t, "yunikorn")
	q, err := provide(in)
	require.Error(t, err)
	assert.Nil(t, q)
	assert.Contains(t, err.Error(), "unrecognized jobs.scheduler.backend")
}
