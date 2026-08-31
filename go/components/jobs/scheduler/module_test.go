package scheduler

import (
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	sched "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework/frameworkmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/kueuemocks"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func setupProvideIn(t *testing.T, backend string) ProvideIn {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	mockClient := fake.NewClientBuilder().WithScheme(scheme).Build()

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
	in := setupProvideIn(t, "kueue")
	q, err := provide(in)
	require.NoError(t, err)
	_, ok := q.(*kueue.KueueJobQueue)
	assert.True(t, ok, "expected *kueue.KueueJobQueue, got %T", q)
}

func TestProvideUnknownBackend(t *testing.T) {
	in := setupProvideIn(t, "yunikorn")
	q, err := provide(in)
	require.Error(t, err)
	assert.Nil(t, q)
	assert.Contains(t, err.Error(), "unrecognized jobs.scheduler.backend")
}
