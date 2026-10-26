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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sched "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/kueuemocks"

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

func TestProvideUnknownBackend(t *testing.T) {
	in := setupProvideIn(t, "yunikorn")
	q, err := provide(in)
	require.Error(t, err)
	assert.Nil(t, q)
	assert.Contains(t, err.Error(), "unrecognized jobs.scheduler.backend")
}
