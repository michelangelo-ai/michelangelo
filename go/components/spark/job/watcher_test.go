package job

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// The watch looks the global SparkJob up in the namespace named by the
// SparkApplication's project label, so the two are deliberately different from
// the local namespace the application itself lives in.
const (
	watcherTestProject   = "test-project"
	watcherTestEnv       = "staging"
	watcherTestFinalizer = "test.michelangelo.ai/hold"
)

// The UIDs the projection's job-UID label names. watcherTestJobUID is the
// stored SparkJob's; watcherTestOtherJobUID is another job's, such as an
// earlier job of the same name whose application is still on the member
// cluster.
const (
	watcherTestJobUID      types.UID = "6f1d3c2e-0b4a-4e8f-9a51-7c2d8e4b1f03"
	watcherTestOtherJobUID types.UID = "a3c9e5f1-2d7b-4c60-8e14-5b9f0d6a2c78"
)

// sparkJobsResource is the resource the control plane's API server names in
// its errors about a SparkJob.
var sparkJobsResource = schema.GroupResource{Group: "michelangelo.api", Resource: "sparkjobs"}

// watcherFixture is a Reconciler whose Client is a fake API server preloaded
// with objects, together with what the tests observe: the mapper mock, the
// number of writes that reached the API server, and the handlers' log entries.
//
// The fake client is used directly rather than through
// testfakes.NewFakeClientWrapper: that wrapper restores the pre-write
// ResourceVersion on the in-memory object, which would hide exactly what these
// tests need to see. Writes are counted with interceptor.Funcs instead, the
// idiom go/components/ray/job/controller_test.go uses, so "the handler left the
// job alone" is asserted on the write itself and not on a derived field.
type watcherFixture struct {
	r      *Reconciler
	mapper *typesmocks.MockMapper
	writes *int
	logs   *observer.ObservedLogs
}

// newWatcherFixture builds a watcherFixture. hooks, when not nil, lets a test
// act inside the fake API server's calls: its Get runs in place of every read,
// and its Update and SubResourceUpdate in place of those writes, which are
// counted either way. A hook stands in for another writer by changing the
// stored job through the client it is handed, which is neither intercepted nor
// counted, and fails a call by returning an error.
func newWatcherFixture(t *testing.T, hooks *interceptor.Funcs, objects ...client.Object) *watcherFixture {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))

	if hooks == nil {
		hooks = &interceptor.Funcs{}
	}
	writes := new(int)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(&v2pb.SparkJob{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: hooks.Get,
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				*writes++
				if hooks.Update != nil {
					return hooks.Update(ctx, c, obj, opts...)
				}
				return c.Update(ctx, obj, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				*writes++
				if hooks.SubResourceUpdate != nil {
					return hooks.SubResourceUpdate(ctx, c, subResource, obj, opts...)
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				*writes++
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	// The handlers log to the test's output as well as to an observer the
	// tests query.
	observedCore, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(zapcore.NewTee(zaptest.NewLogger(t).Core(), observedCore))

	// No EXPECT is registered here: a handler that must not consult the mapper
	// is asserted by gomock failing the test on the unexpected call.
	mapper := typesmocks.NewMockMapper(gomock.NewController(t))

	return &watcherFixture{
		r: &Reconciler{
			Client: fakeClient,
			env:    env.Context{RuntimeEnvironment: watcherTestEnv},
			logger: zapr.NewLogger(logger),
			mapper: mapper,
		},
		mapper: mapper,
		writes: writes,
		logs:   logs,
	}
}

// newWatcherTestReconciler is newWatcherFixture without hooks, for the tests
// that need only the Reconciler, the mapper mock and the write count.
func newWatcherTestReconciler(t *testing.T, objects ...client.Object) (*Reconciler, *typesmocks.MockMapper, *int) {
	t.Helper()
	f := newWatcherFixture(t, nil, objects...)
	return f.r, f.mapper, f.writes
}

// projectForReal replaces the mapper mock with the production projection,
// k8sengine.Mapper, so the handlers read the application's state exactly as
// they do in production.
func (f *watcherFixture) projectForReal() {
	f.r.mapper = k8sengine.Mapper{}
}

// errorLogs returns the entries the handlers logged at Error level.
func (f *watcherFixture) errorLogs() []observer.LoggedEntry {
	return f.logs.FilterLevelExact(zapcore.ErrorLevel).All()
}

// assertErrorLogged asserts that the handler logged exactly one error and that
// the entry says what went wrong, or, when want is false, that it logged none.
func (f *watcherFixture) assertErrorLogged(t *testing.T, want bool) {
	t.Helper()
	entries := f.errorLogs()
	if !want {
		assert.Empty(t, entries, "nothing went wrong that an error log should report")
		return
	}
	if assert.Len(t, entries, 1) {
		assert.NotEmpty(t, entries[0].ContextMap()["error"], "an error log must say what went wrong")
	}
}

// watcherTestSparkApplication builds the SparkApplication the control plane
// projects for the stored SparkJob: local namespace, global name, and the
// labels the selector and the handlers read, among them the job-UID label,
// which names watcherTestJobUID as the projection names the job's UID on every
// application it creates. labeledSparkApplication names another UID, and
// unlabeledSparkApplication leaves the label off.
func watcherTestSparkApplication(state sparkv1beta2.ApplicationStateType) *sparkv1beta2.SparkApplication {
	return &sparkv1beta2.SparkApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sparkJobName,
			Namespace: k8sengine.SparkLocalNamespace,
			Labels: map[string]string{
				constants.ProjectNameLabelKey:   watcherTestProject,
				constants.OwnerServiceLabelKey:  constants.MAOwnerSparkLabelValue,
				constants.JobControlPlaneEnvKey: watcherTestEnv,
				constants.JobUIDLabelKey:        string(watcherTestJobUID),
			},
		},
		Status: sparkv1beta2.SparkApplicationStatus{
			AppState: sparkv1beta2.ApplicationState{State: state},
		},
	}
}

// labeledSparkApplication builds the SparkApplication the projection makes for
// the job with UID jobUID: watcherTestSparkApplication with a job-UID label
// that names jobUID.
func labeledSparkApplication(state sparkv1beta2.ApplicationStateType, jobUID types.UID) *sparkv1beta2.SparkApplication {
	local := watcherTestSparkApplication(state)
	local.Labels[constants.JobUIDLabelKey] = string(jobUID)
	return local
}

// unlabeledSparkApplication builds watcherTestSparkApplication without its
// job-UID label: an application the control plane did not create for any job,
// which still carries the labels the watch selects on, so that only the
// ownership check stands between its events and the job.
func unlabeledSparkApplication(state sparkv1beta2.ApplicationStateType) *sparkv1beta2.SparkApplication {
	local := watcherTestSparkApplication(state)
	delete(local.Labels, constants.JobUIDLabelKey)
	return local
}

// watcherTestSparkJob builds the global SparkJob the handlers resolve to.
func watcherTestSparkJob(conditions ...*apipb.Condition) *v2pb.SparkJob {
	return &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sparkJobName,
			Namespace: watcherTestProject,
			UID:       watcherTestJobUID,
		},
		Status: v2pb.SparkJobStatus{StatusConditions: conditions},
	}
}

// scheduledWatcherTestSparkJob builds the global SparkJob of an application
// about to be launched: the scheduler has assigned it to testClusterName, and
// the controller has not recorded the launch yet.
func scheduledWatcherTestSparkJob(conditions ...*apipb.Condition) *v2pb.SparkJob {
	job := watcherTestSparkJob(append([]*apipb.Condition{
		watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, constants.AddedToSchedulerQueue),
		watcherTestCondition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE, ""),
	}, conditions...)...)
	job.Status.Assignment = &v2pb.AssignmentInfo{Cluster: testClusterName}
	return job
}

// launchedWatcherTestSparkJob is scheduledWatcherTestSparkJob once the
// controller has launched the application, before the watch reports on it.
func launchedWatcherTestSparkJob(conditions ...*apipb.Condition) *v2pb.SparkJob {
	return scheduledWatcherTestSparkJob(append([]*apipb.Condition{
		watcherTestCondition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE, reasonSparkApplicationCreated),
	}, conditions...)...)
}

// runningWatcherTestSparkJob is launchedWatcherTestSparkJob as the watch
// records a running application: Succeeded UNKNOWN and SparkAppRunning TRUE,
// both in state RUNNING.
func runningWatcherTestSparkJob() *v2pb.SparkJob {
	return launchedWatcherTestSparkJob(
		watcherTestCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN, string(sparkv1beta2.RunningState)),
		watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState)),
	)
}

func watcherTestCondition(conditionType string, status apipb.ConditionStatus, reason string) *apipb.Condition {
	return &apipb.Condition{Type: conditionType, Status: status, Reason: reason}
}

// getWatcherTestSparkJob reads the stored global SparkJob back.
func getWatcherTestSparkJob(t *testing.T, r *Reconciler) *v2pb.SparkJob {
	t.Helper()

	var sparkJob v2pb.SparkJob
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Namespace: watcherTestProject, Name: sparkJobName}, &sparkJob))
	return &sparkJob
}

// readStoredSparkJob reads the stored global SparkJob through c, the client a
// hook is handed.
func readStoredSparkJob(t *testing.T, ctx context.Context, c client.Client) *v2pb.SparkJob {
	t.Helper()

	var sparkJob v2pb.SparkJob
	require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: watcherTestProject, Name: sparkJobName}, &sparkJob))
	return &sparkJob
}

// writeStoredCondition writes condition onto the stored global SparkJob through
// c, standing in for another writer whose write lands in the middle of a
// handler's.
func writeStoredCondition(t *testing.T, ctx context.Context, c client.Client, condition *apipb.Condition) {
	t.Helper()

	current := readStoredSparkJob(t, ctx, c)
	jobsutils.UpdateCondition(
		jobsutils.GetCondition(&current.Status.StatusConditions, condition.Type, current.Generation),
		jobsutils.ConditionUpdateParams{
			Status:     condition.Status,
			Reason:     condition.Reason,
			Message:    condition.Message,
			Generation: current.Generation,
		})
	require.NoError(t, c.Status().Update(ctx, current))
}

// failingFirstRead returns hooks that fail the first read of the job, the
// handler's own, with a transient error and let every later one through.
func failingFirstRead() *interceptor.Funcs {
	reads := 0
	return &interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			reads++
			if reads == 1 {
				return unavailableError()
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}

// conflictError is the API server's answer to a write made against a stale
// resourceVersion.
func conflictError() error {
	return apiErrors.NewConflict(sparkJobsResource, sparkJobName,
		errors.New("the object has been modified; please apply your changes to the latest version and try again"))
}

// unavailableError is a transient failure: the API server is up but cannot
// serve the request right now.
func unavailableError() error {
	return apiErrors.NewServiceUnavailable("the server is currently unable to handle the request")
}

// forbiddenError is a failure that neither a fresh read nor waiting can fix.
func forbiddenError() error {
	return apiErrors.NewForbidden(sparkJobsResource, sparkJobName,
		errors.New("the controller manager may not update spark jobs"))
}

// watcherTestHandler is one of the watch's event handlers, driven with the
// application an event carries.
type watcherTestHandler struct {
	name   string
	invoke func(r *Reconciler, local *sparkv1beta2.SparkApplication)
}

var (
	watcherTestAdd = watcherTestHandler{
		name:   "add",
		invoke: func(r *Reconciler, local *sparkv1beta2.SparkApplication) { r.sparkApplicationAddEventHandler(local) },
	}
	watcherTestUpdate = watcherTestHandler{
		name: "update",
		invoke: func(r *Reconciler, local *sparkv1beta2.SparkApplication) {
			r.sparkApplicationUpdateEventHandler(nil, local)
		},
	}
	watcherTestDelete = watcherTestHandler{
		name:   "delete",
		invoke: func(r *Reconciler, local *sparkv1beta2.SparkApplication) { r.sparkApplicationDeleteEventHandler(local) },
	}
	// watcherTestHandlers is every handler the watch registers.
	watcherTestHandlers = []watcherTestHandler{watcherTestAdd, watcherTestUpdate, watcherTestDelete}
)

// TestSparkApplicationWatcherParams covers the watch description: what it
// watches, where, and -- the part a wrong answer would make the control plane
// act on other people's applications -- which ones it accepts.
func TestSparkApplicationWatcherParams(t *testing.T) {
	r := &Reconciler{env: env.Context{RuntimeEnvironment: watcherTestEnv}}
	params := r.sparkApplicationWatcherParams()

	assert.Equal(t, constants.KubeSparkResource, params.ResourceName)
	assert.Equal(t, k8sengine.SparkLocalNamespace, params.Namespace)
	assert.IsType(t, &sparkv1beta2.SparkApplication{}, params.ObjType)

	handlers, ok := params.ResourceEventHandler.(cache.ResourceEventHandlerFuncs)
	require.True(t, ok, "event handler should be cache.ResourceEventHandlerFuncs")
	assert.NotNil(t, handlers.AddFunc)
	assert.NotNil(t, handlers.UpdateFunc)
	assert.NotNil(t, handlers.DeleteFunc)

	selector, err := metav1.LabelSelectorAsSelector(params.LabelSelector)
	require.NoError(t, err)

	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name: "spark application from this control plane and environment",
			labels: map[string]string{
				constants.OwnerServiceLabelKey:  constants.MAOwnerSparkLabelValue,
				constants.JobControlPlaneEnvKey: watcherTestEnv,
			},
			want: true,
		},
		{
			name: "ray workload from this control plane",
			labels: map[string]string{
				constants.OwnerServiceLabelKey:  constants.MAOwnerServiceLabelValue,
				constants.JobControlPlaneEnvKey: watcherTestEnv,
			},
			want: false,
		},
		{
			name: "spark application from another environment",
			labels: map[string]string{
				constants.OwnerServiceLabelKey:  constants.MAOwnerSparkLabelValue,
				constants.JobControlPlaneEnvKey: "production",
			},
			want: false,
		},
		{
			name:   "spark application this control plane did not project",
			labels: map[string]string{},
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, selector.Matches(labels.Set(tc.labels)))
		})
	}
}

// TestSparkApplicationWatcherParams_HandlerWiring drives each handler the
// watch registers and checks that it does its own job. The add and delete
// handlers have the same signature, so wiring one in the other's place would
// compile; only what they do tells them apart.
func TestSparkApplicationWatcherParams_HandlerWiring(t *testing.T) {
	registered := func(t *testing.T, f *watcherFixture) cache.ResourceEventHandlerFuncs {
		t.Helper()
		handlers, ok := f.r.sparkApplicationWatcherParams().ResourceEventHandler.(cache.ResourceEventHandlerFuncs)
		require.True(t, ok, "event handler should be cache.ResourceEventHandlerFuncs")
		return handlers
	}
	// assertSyncedRunning asserts the job records the application as running,
	// which is what an add or update event for a RUNNING application writes.
	assertSyncedRunning := func(t *testing.T, f *watcherFixture) {
		t.Helper()
		assert.Equal(t, 1, *f.writes, "a status sync is a single write")
		got := getWatcherTestSparkJob(t, f.r)
		succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded)
		assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, succeeded.Status)
		assert.Equal(t, string(sparkv1beta2.RunningState), succeeded.Reason)
		assert.False(t, apiutils.IsImmutable(got), "a status sync does not freeze the job")
	}

	t.Run("AddFunc syncs the application's status", func(t *testing.T) {
		f := newWatcherFixture(t, nil, launchedWatcherTestSparkJob())
		f.projectForReal()

		registered(t, f).AddFunc(watcherTestSparkApplication(sparkv1beta2.RunningState))

		assertSyncedRunning(t, f)
	})

	t.Run("UpdateFunc syncs the new application, not the old one", func(t *testing.T) {
		f := newWatcherFixture(t, nil, launchedWatcherTestSparkJob())
		f.projectForReal()

		registered(t, f).UpdateFunc(
			watcherTestSparkApplication(sparkv1beta2.SubmittedState),
			watcherTestSparkApplication(sparkv1beta2.RunningState))

		assertSyncedRunning(t, f)
	})

	t.Run("DeleteFunc records the deletion", func(t *testing.T) {
		f := newWatcherFixture(t, nil, launchedWatcherTestSparkJob())
		f.projectForReal()

		registered(t, f).DeleteFunc(watcherTestSparkApplication(sparkv1beta2.RunningState))

		assert.Equal(t, 2, *f.writes, "one status write for the record, one plain write for the freeze")
		got := getWatcherTestSparkJob(t, f.r)
		succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
		require.NotNil(t, succeeded)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
		assert.Equal(t, reasonSparkApplicationDeleted, succeeded.Reason)
		assert.True(t, apiutils.IsImmutable(got))
	})
}

// TestSparkApplicationEventHandler_SyncsStatus is the happy path: the mapper's
// projection lands on the global SparkJob, and the conditions the scheduler
// owns survive it.
func TestSparkApplicationEventHandler_SyncsStatus(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(r *Reconciler, local *sparkv1beta2.SparkApplication)
	}{
		{
			name:   "add event",
			invoke: func(r *Reconciler, local *sparkv1beta2.SparkApplication) { r.sparkApplicationAddEventHandler(local) },
		},
		{
			name: "update event",
			invoke: func(r *Reconciler, local *sparkv1beta2.SparkApplication) {
				r.sparkApplicationUpdateEventHandler(nil, local)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			local := watcherTestSparkApplication(sparkv1beta2.RunningState)
			stored := watcherTestSparkJob(
				watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, constants.AddedToSchedulerQueue),
				watcherTestCondition(constants.ScheduledCondition, apipb.CONDITION_STATUS_TRUE, ""),
			)
			r, mapper, writes := newWatcherTestReconciler(t, stored)

			mapper.EXPECT().MapLocalJobStatusToGlobal(local).Return(&matypes.JobStatus{
				Spark: &v2pb.SparkJobStatus{
					ApplicationId: "spark-application-1",
					JobUrl:        "http://driver/ui",
					StatusConditions: []*apipb.Condition{
						watcherTestCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN, string(sparkv1beta2.RunningState)),
						watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState)),
					},
				},
			}, nil).Times(1)

			tc.invoke(r, local)

			assert.Equal(t, 1, *writes, "handler should write the global spark job exactly once")

			got := getWatcherTestSparkJob(t, r)
			assert.Equal(t, "spark-application-1", got.Status.ApplicationId)
			assert.Equal(t, "http://driver/ui", got.Status.JobUrl)

			succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, succeeded.Status)
			assert.Equal(t, string(sparkv1beta2.RunningState), succeeded.Reason)

			running := findCondition(got.Status.StatusConditions, constants.SparkAppRunningCondition)
			require.NotNil(t, running)
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, running.Status)

			// The watch owns the application conditions, not the whole list.
			enqueued := findCondition(got.Status.StatusConditions, constants.EnqueuedCondition)
			require.NotNil(t, enqueued, "scheduler's Enqueued condition should survive the sync")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, enqueued.Status)
			assert.Equal(t, constants.AddedToSchedulerQueue, enqueued.Reason)

			scheduled := findCondition(got.Status.StatusConditions, constants.ScheduledCondition)
			require.NotNil(t, scheduled, "scheduler's Scheduled condition should survive the sync")
			assert.Equal(t, apipb.CONDITION_STATUS_TRUE, scheduled.Status)
		})
	}
}

// TestSparkApplicationEventHandler_NeverClearsConditions pins the merge's
// upsert semantics: the projection stops emitting SparkAppRunning once the
// application leaves RUNNING, and the stored condition is history, not a claim
// about the present, so it stays.
func TestSparkApplicationEventHandler_NeverClearsConditions(t *testing.T) {
	local := watcherTestSparkApplication(sparkv1beta2.CompletedState)
	stored := watcherTestSparkJob(
		watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState)),
	)
	r, mapper, writes := newWatcherTestReconciler(t, stored)

	mapper.EXPECT().MapLocalJobStatusToGlobal(local).Return(&matypes.JobStatus{
		Spark: &v2pb.SparkJobStatus{
			StatusConditions: []*apipb.Condition{
				watcherTestCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState)),
			},
		},
	}, nil).Times(1)

	r.sparkApplicationAddEventHandler(local)

	assert.Equal(t, 1, *writes)

	got := getWatcherTestSparkJob(t, r)
	succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
	require.NotNil(t, succeeded)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeeded.Status)

	running := findCondition(got.Status.StatusConditions, constants.SparkAppRunningCondition)
	require.NotNil(t, running, "an omitted condition must be left alone, not cleared")
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, running.Status)
}

// TestSparkApplicationEventHandler_NoChangeNoWrite covers the cache re-sync:
// an Update event carrying nothing new must not produce an API write.
// TestMergeSparkApplicationStatus pins the other half, that the check leaves
// the status it is given exactly as it was; without a write the stored job
// cannot change, so asserting on it here would prove nothing.
func TestSparkApplicationEventHandler_NoChangeNoWrite(t *testing.T) {
	local := watcherTestSparkApplication(sparkv1beta2.RunningState)
	stored := watcherTestSparkJob(
		watcherTestCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN, string(sparkv1beta2.RunningState)),
		watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState)),
	)
	stored.Status.ApplicationId = "spark-application-1"
	stored.Status.JobUrl = "http://driver/ui"
	r, mapper, writes := newWatcherTestReconciler(t, stored)

	mapper.EXPECT().MapLocalJobStatusToGlobal(local).Return(&matypes.JobStatus{
		Spark: &v2pb.SparkJobStatus{
			ApplicationId: "spark-application-1",
			JobUrl:        "http://driver/ui",
			StatusConditions: []*apipb.Condition{
				watcherTestCondition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN, string(sparkv1beta2.RunningState)),
				watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState)),
			},
		},
	}, nil).Times(1)

	r.sparkApplicationUpdateEventHandler(local, local)

	assert.Equal(t, 0, *writes, "an unchanged application should not be written back")
}

// TestSparkApplicationEventHandler_TransitionsStoredConditions covers events
// that change a condition the job already has, which is most of them: every
// state before the outcome is Succeeded UNKNOWN with only its reason moving,
// and the identifiers arrive while the conditions stand still. Each event is
// written exactly once; the condition it changes is rewritten and restamped,
// and what it leaves alone stays as it was, down to the timestamp. An
// identifier the projection leaves empty keeps the stored one.
func TestSparkApplicationEventHandler_TransitionsStoredConditions(t *testing.T) {
	const (
		applicationID = "spark-application-1"
		jobURL        = "http://driver/ui"
		// stamp is when the stored conditions were written, long enough ago for
		// a rewrite to restamp them visibly.
		stamp int64 = 1_700_000_000
	)
	succeeded := func(status apipb.ConditionStatus, state sparkv1beta2.ApplicationStateType, message string) *apipb.Condition {
		return &apipb.Condition{Type: constants.SucceededCondition, Status: status, Reason: string(state), Message: message}
	}
	running := func() *apipb.Condition {
		return succeeded(apipb.CONDITION_STATUS_UNKNOWN, sparkv1beta2.RunningState, "")
	}
	tests := []struct {
		name                      string
		stored                    *apipb.Condition
		storedID, storedURL       string
		projected                 *apipb.Condition
		projectedID, projectedURL string
		// wantWrite is whether the event is written, exactly once, or not at all.
		wantWrite       bool
		wantID, wantURL string
	}{
		{
			name:      "the reason moves on",
			stored:    succeeded(apipb.CONDITION_STATUS_UNKNOWN, sparkv1beta2.SubmittedState, ""),
			projected: running(),
			wantWrite: true,
		},
		{
			name:      "the application completes",
			stored:    running(),
			projected: succeeded(apipb.CONDITION_STATUS_TRUE, sparkv1beta2.CompletedState, ""),
			wantWrite: true,
		},
		{
			name:      "the application fails",
			stored:    running(),
			projected: succeeded(apipb.CONDITION_STATUS_FALSE, sparkv1beta2.FailedState, "driver container failed with exit code 1"),
			wantWrite: true,
		},
		{
			name:      "only the message changes",
			stored:    running(),
			projected: succeeded(apipb.CONDITION_STATUS_UNKNOWN, sparkv1beta2.RunningState, "executor 2 was lost and is being replaced"),
			wantWrite: true,
		},
		{
			// An earlier projection left a failed submission unresolved, and the
			// current one reads the same state and message as a failure.
			name:      "only the status changes",
			stored:    succeeded(apipb.CONDITION_STATUS_UNKNOWN, sparkv1beta2.FailedSubmissionState, "failed to run spark-submit: exit status 1"),
			projected: succeeded(apipb.CONDITION_STATUS_FALSE, sparkv1beta2.FailedSubmissionState, "failed to run spark-submit: exit status 1"),
			wantWrite: true,
		},
		{
			name:        "only the application id appears",
			stored:      running(),
			projected:   running(),
			projectedID: applicationID,
			wantWrite:   true,
			wantID:      applicationID,
		},
		{
			name:         "only the job url appears",
			stored:       running(),
			projected:    running(),
			projectedURL: jobURL,
			wantWrite:    true,
			wantURL:      jobURL,
		},
		{
			name:         "an empty application id keeps the stored one",
			stored:       running(),
			storedID:     applicationID,
			storedURL:    jobURL,
			projected:    running(),
			projectedURL: jobURL,
			wantID:       applicationID,
			wantURL:      jobURL,
		},
		{
			name:        "an empty job url keeps the stored one",
			stored:      running(),
			storedID:    applicationID,
			storedURL:   jobURL,
			projected:   running(),
			projectedID: applicationID,
			wantID:      applicationID,
			wantURL:     jobURL,
		},
		{
			name:      "empty identifiers keep the stored ones while the application completes",
			stored:    running(),
			storedID:  applicationID,
			storedURL: jobURL,
			projected: succeeded(apipb.CONDITION_STATUS_TRUE, sparkv1beta2.CompletedState, ""),
			wantWrite: true,
			wantID:    applicationID,
			wantURL:   jobURL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			local := watcherTestSparkApplication(sparkv1beta2.RunningState)
			tc.stored.LastUpdatedTimestamp = stamp
			sibling := watcherTestCondition(constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.RunningState))
			sibling.LastUpdatedTimestamp = stamp
			stored := watcherTestSparkJob(tc.stored, sibling)
			stored.Status.ApplicationId = tc.storedID
			stored.Status.JobUrl = tc.storedURL
			r, mapper, writes := newWatcherTestReconciler(t, stored)

			mapper.EXPECT().MapLocalJobStatusToGlobal(local).Return(&matypes.JobStatus{
				Spark: &v2pb.SparkJobStatus{
					ApplicationId:    tc.projectedID,
					JobUrl:           tc.projectedURL,
					StatusConditions: []*apipb.Condition{tc.projected},
				},
			}, nil).Times(1)

			r.sparkApplicationUpdateEventHandler(nil, local)

			wantWrites := 0
			if tc.wantWrite {
				wantWrites = 1
			}
			assert.Equal(t, wantWrites, *writes)

			got := getWatcherTestSparkJob(t, r)
			assert.Equal(t, tc.wantID, got.Status.ApplicationId)
			assert.Equal(t, tc.wantURL, got.Status.JobUrl)
			require.Len(t, got.Status.StatusConditions, 2, "no condition may be added or dropped")

			gotSucceeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, gotSucceeded)
			assert.Equal(t, tc.projected.Status, gotSucceeded.Status)
			assert.Equal(t, tc.projected.Reason, gotSucceeded.Reason)
			assert.Equal(t, tc.projected.Message, gotSucceeded.Message)
			conditionChanged := tc.stored.Status != tc.projected.Status ||
				tc.stored.Reason != tc.projected.Reason ||
				tc.stored.Message != tc.projected.Message
			if conditionChanged {
				assert.Greater(t, gotSucceeded.LastUpdatedTimestamp, stamp, "a rewritten condition is restamped")
			} else {
				assert.Equal(t, stamp, gotSucceeded.LastUpdatedTimestamp, "an unchanged condition is not rewritten")
			}

			gotSibling := findCondition(got.Status.StatusConditions, constants.SparkAppRunningCondition)
			require.NotNil(t, gotSibling)
			assert.Equal(t, stamp, gotSibling.LastUpdatedTimestamp, "a condition the event does not change is not rewritten")
		})
	}
}

// TestMergeSparkApplicationStatus pins the merge where the handlers' no-op
// check depends on it: a projection that changes nothing reports no change and
// leaves the status exactly as it was -- no placeholder condition, no new
// timestamp, no new generation -- and an identifier the projection leaves
// empty is kept rather than cleared.
func TestMergeSparkApplicationStatus(t *testing.T) {
	const stamp int64 = 1_700_000_000
	// stored is the status as last written, at generation 1.
	stored := func() *v2pb.SparkJobStatus {
		return &v2pb.SparkJobStatus{
			ApplicationId: "spark-application-1",
			JobUrl:        "http://driver/ui",
			StatusConditions: []*apipb.Condition{
				{
					Type:                 constants.SucceededCondition,
					Status:               apipb.CONDITION_STATUS_UNKNOWN,
					Reason:               string(sparkv1beta2.RunningState),
					ObservedGeneration:   1,
					LastUpdatedTimestamp: stamp,
				},
				{
					Type:                 constants.SparkAppRunningCondition,
					Status:               apipb.CONDITION_STATUS_TRUE,
					Reason:               string(sparkv1beta2.RunningState),
					ObservedGeneration:   1,
					LastUpdatedTimestamp: stamp,
				},
			},
		}
	}

	t.Run("a projection that changes nothing", func(t *testing.T) {
		dst := stored()
		// The job's generation has moved on since: a condition the event does
		// not change keeps the generation it was written at.
		assert.False(t, mergeSparkApplicationStatus(dst, stored(), 2))
		assert.Equal(t, stored(), dst)
	})

	t.Run("a projection without identifiers keeps the stored ones", func(t *testing.T) {
		dst := stored()
		projected := stored()
		projected.ApplicationId = ""
		projected.JobUrl = ""
		assert.False(t, mergeSparkApplicationStatus(dst, projected, 1))
		assert.Equal(t, stored(), dst)
		assert.Empty(t, projected.ApplicationId, "the projection itself is left alone")
		assert.Empty(t, projected.JobUrl, "the projection itself is left alone")
	})

	t.Run("new identifiers replace the stored ones", func(t *testing.T) {
		dst := stored()
		projected := stored()
		projected.ApplicationId = "spark-application-2"
		projected.JobUrl = "http://driver-2/ui"
		assert.True(t, mergeSparkApplicationStatus(dst, projected, 1))
		assert.Equal(t, "spark-application-2", dst.ApplicationId)
		assert.Equal(t, "http://driver-2/ui", dst.JobUrl)
		assert.Equal(t, stored().StatusConditions, dst.StatusConditions, "the conditions did not change")
	})
}

// TestSparkApplicationEventHandler_Guards covers the states in which the watch
// must keep its hands off the global job, for every handler: the job is
// frozen, being deleted, being terminated or finished -- its outcome recorded
// by the watch or by the controller -- or the application is not its own.
// Each case also asserts the mapper was never consulted: gomock fails on a
// call with no EXPECT.
func TestSparkApplicationEventHandler_Guards(t *testing.T) {
	enqueued := func() *apipb.Condition {
		return watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, "")
	}
	tests := []struct {
		name  string
		setup func(t *testing.T) *v2pb.SparkJob
		// prepare runs against the live client for states the fake client will
		// not accept on a freshly created object.
		prepare func(t *testing.T, r *Reconciler, job *v2pb.SparkJob)
		// local is the application the event carries; the job's own RUNNING one
		// when nil.
		local func() *sparkv1beta2.SparkApplication
	}{
		{
			name: "job is immutable",
			setup: func(t *testing.T) *v2pb.SparkJob {
				job := watcherTestSparkJob(enqueued())
				apiutils.MarkImmutable(job)
				return job
			},
		},
		{
			name: "job is being deleted",
			setup: func(t *testing.T) *v2pb.SparkJob {
				job := watcherTestSparkJob(enqueued())
				// The fake client rejects an object created with a deletion
				// timestamp, so the timestamp is set by a real Delete and a
				// finalizer keeps the object around to be read back.
				job.Finalizers = []string{watcherTestFinalizer}
				return job
			},
			prepare: func(t *testing.T, r *Reconciler, job *v2pb.SparkJob) {
				require.NoError(t, r.Delete(context.Background(), job))
				require.NotNil(t, getWatcherTestSparkJob(t, r).GetDeletionTimestamp())
			},
		},
		{
			name: "job is marked as being deleted",
			setup: func(t *testing.T) *v2pb.SparkJob {
				// The deleting annotation alone, without a deletion timestamp: the
				// API server's delete path marks the job and leaves the delete
				// itself to the ingester.
				job := watcherTestSparkJob(enqueued())
				job.Annotations = map[string]string{api.DeletingAnnotation: "true"}
				return job
			},
		},
		{
			name: "termination was requested",
			setup: func(t *testing.T) *v2pb.SparkJob {
				job := watcherTestSparkJob(enqueued())
				job.Spec.Termination = &v2pb.TerminationSpec{Type: v2pb.TERMINATION_TYPE_FAILED}
				return job
			},
		},
		{
			name: "outcome is recorded: the application completed",
			setup: func(t *testing.T) *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState)))
			},
		},
		{
			name: "outcome is recorded: the application failed",
			setup: func(t *testing.T) *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE, string(sparkv1beta2.FailedState)))
			},
		},
		{
			name: "outcome is recorded: the controller could not create the application",
			setup: func(t *testing.T) *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE, reasonSparkApplicationCreationFailed))
			},
		},
		{
			name: "outcome is recorded: the job was killed",
			setup: func(t *testing.T) *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.KilledCondition, apipb.CONDITION_STATUS_TRUE, constants.SparkAppKilled))
			},
		},
		{
			name: "application belongs to another job",
			setup: func(t *testing.T) *v2pb.SparkJob {
				return watcherTestSparkJob(enqueued())
			},
			local: func() *sparkv1beta2.SparkApplication {
				return labeledSparkApplication(sparkv1beta2.RunningState, watcherTestOtherJobUID)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, handler := range watcherTestHandlers {
				t.Run(handler.name, func(t *testing.T) {
					local := watcherTestSparkApplication(sparkv1beta2.RunningState)
					if tc.local != nil {
						local = tc.local()
					}
					job := tc.setup(t)
					conditionCount := len(job.Status.StatusConditions)
					wasImmutable := apiutils.IsImmutable(job)
					r, _, writes := newWatcherTestReconciler(t, job)
					if tc.prepare != nil {
						tc.prepare(t, r, getWatcherTestSparkJob(t, r))
						*writes = 0
					}

					handler.invoke(r, local)

					assert.Equal(t, 0, *writes, "guarded job should not be written")

					got := getWatcherTestSparkJob(t, r)
					assert.Empty(t, got.Status.ApplicationId)
					assert.Len(t, got.Status.StatusConditions, conditionCount, "guarded job's conditions should be untouched")
					assert.Equal(t, wasImmutable, apiutils.IsImmutable(got), "the watch does not freeze a job it leaves alone")
				})
			}
		})
	}
}

// TestSparkApplicationEventHandlers_Ownership covers the job-UID label the
// projection puts on every application. Applications are named after their
// job in a namespace that every project shares on a member cluster, so the
// application under this job's name may not be this job's: another job of the
// same name may have left it behind, or something other than the control plane
// may have created it. Only an application whose label names the job's UID is
// the job's own; one without the label, or with an empty one, is not, and a job
// without a UID owns none. Each handler skips the events of an application
// that is not the job's own in its first check, says why at V(1) and writes
// nothing. The real projection is wired in only for the job's own application,
// so a handler that went on to map another application's status would fail the
// test on the mapper mock, which expects no call.
func TestSparkApplicationEventHandlers_Ownership(t *testing.T) {
	// skipMessage is what each handler logs when it leaves the job alone.
	skipMessage := map[string]string{
		watcherTestAdd.name:    "skipping status update for spark job",
		watcherTestUpdate.name: "skipping status update for spark job",
		watcherTestDelete.name: "skipping delete event for spark job",
	}
	applications := []struct {
		name string
		// jobUID is the stored job's UID.
		jobUID types.UID
		local  func() *sparkv1beta2.SparkApplication
		owned  bool
	}{
		{
			name:   "labeled with this job's UID",
			jobUID: watcherTestJobUID,
			local: func() *sparkv1beta2.SparkApplication {
				return labeledSparkApplication(sparkv1beta2.RunningState, watcherTestJobUID)
			},
			owned: true,
		},
		{
			name:   "without the label",
			jobUID: watcherTestJobUID,
			local:  func() *sparkv1beta2.SparkApplication { return unlabeledSparkApplication(sparkv1beta2.RunningState) },
		},
		{
			name:   "with an empty label",
			jobUID: watcherTestJobUID,
			local:  func() *sparkv1beta2.SparkApplication { return labeledSparkApplication(sparkv1beta2.RunningState, "") },
		},
		{
			name:   "labeled with another job's UID",
			jobUID: watcherTestJobUID,
			local: func() *sparkv1beta2.SparkApplication {
				return labeledSparkApplication(sparkv1beta2.RunningState, watcherTestOtherJobUID)
			},
		},
		{
			// A rule that only compared the label with the UID would take an
			// empty label for a match here.
			name:   "with an empty label, for a job without a UID",
			jobUID: "",
			local:  func() *sparkv1beta2.SparkApplication { return labeledSparkApplication(sparkv1beta2.RunningState, "") },
		},
	}
	for _, app := range applications {
		t.Run(app.name, func(t *testing.T) {
			for _, handler := range watcherTestHandlers {
				t.Run(handler.name, func(t *testing.T) {
					job := launchedWatcherTestSparkJob()
					job.UID = app.jobUID
					f := newWatcherFixture(t, nil, job)
					if app.owned {
						f.projectForReal()
					}

					handler.invoke(f.r, app.local())

					got := getWatcherTestSparkJob(t, f.r)
					succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
					skips := f.logs.FilterMessage(skipMessage[handler.name]).All()
					if !app.owned {
						assert.Equal(t, 0, *f.writes, "an application that is not the job's own must not touch it")
						assert.Nil(t, succeeded)
						assert.False(t, apiutils.IsImmutable(got))
						if assert.Len(t, skips, 1, "the event is skipped once, in the handler's first check") {
							assert.Equal(t, zapcore.DebugLevel, skips[0].Level, "a skipped event is logged at V(1)")
							assert.Equal(t, "spark application is not this spark job's own", skips[0].ContextMap()["reason"])
						}
						return
					}
					assert.Empty(t, skips, "the job's own application is not skipped")
					require.NotNil(t, succeeded, "the job's own application is processed")
					if handler.name == watcherTestDelete.name {
						assert.Equal(t, 2, *f.writes, "the record and the freeze")
						assert.Equal(t, reasonSparkApplicationDeleted, succeeded.Reason)
						assert.True(t, apiutils.IsImmutable(got))
					} else {
						assert.Equal(t, 1, *f.writes)
						assert.Equal(t, string(sparkv1beta2.RunningState), succeeded.Reason)
					}
				})
			}
		})
	}
}

// TestSparkApplicationEventHandlers_GuardsAreRecheckedBeforeTheWrite covers
// the window between a handler's first read of the job and its write. Each
// case changes the stored job just before the read inside the write attempt,
// the handler's second, and that read alone must stop the write. The
// application is labeled with the job's UID, so a job replaced by a new one of
// the same name is recognised as no longer its job.
func TestSparkApplicationEventHandlers_GuardsAreRecheckedBeforeTheWrite(t *testing.T) {
	tests := []struct {
		name string
		// finalizer keeps the job readable once its deletion has started.
		finalizer bool
		change    func(t *testing.T, ctx context.Context, c client.Client)
		// outcome is whether the change records Succeeded TRUE / COMPLETED.
		outcome bool
	}{
		{
			name: "the job was frozen",
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				current := readStoredSparkJob(t, ctx, c)
				apiutils.MarkImmutable(current)
				require.NoError(t, c.Update(ctx, current))
			},
		},
		{
			name:      "the job's deletion started",
			finalizer: true,
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				require.NoError(t, c.Delete(ctx, readStoredSparkJob(t, ctx, c)))
			},
		},
		{
			name: "the job was marked as being deleted",
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				current := readStoredSparkJob(t, ctx, c)
				current.Annotations = map[string]string{api.DeletingAnnotation: "true"}
				require.NoError(t, c.Update(ctx, current))
			},
		},
		{
			name: "a termination was requested",
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				current := readStoredSparkJob(t, ctx, c)
				current.Spec.Termination = &v2pb.TerminationSpec{Type: v2pb.TERMINATION_TYPE_FAILED}
				require.NoError(t, c.Update(ctx, current))
			},
		},
		{
			name: "an outcome was recorded",
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				writeStoredCondition(t, ctx, c, watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState)))
			},
			outcome: true,
		},
		{
			name: "the job was replaced by another of the same name",
			change: func(t *testing.T, ctx context.Context, c client.Client) {
				require.NoError(t, c.Delete(ctx, readStoredSparkJob(t, ctx, c)))
				replacement := watcherTestSparkJob()
				replacement.UID = watcherTestOtherJobUID
				require.NoError(t, c.Create(ctx, replacement))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, handler := range watcherTestHandlers {
				t.Run(handler.name, func(t *testing.T) {
					job := launchedWatcherTestSparkJob()
					if tc.finalizer {
						job.Finalizers = []string{watcherTestFinalizer}
					}
					reads := 0
					f := newWatcherFixture(t, &interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							reads++
							if reads == 2 {
								tc.change(t, ctx, c)
							}
							return c.Get(ctx, key, obj, opts...)
						},
					}, job)
					f.projectForReal()

					handler.invoke(f.r, labeledSparkApplication(sparkv1beta2.RunningState, watcherTestJobUID))

					require.GreaterOrEqual(t, reads, 2, "the handler must read the job again before it writes")
					assert.Equal(t, 0, *f.writes, "the fresh read must stop the write")

					got := getWatcherTestSparkJob(t, f.r)
					assert.Nil(t, findCondition(got.Status.StatusConditions, constants.SparkAppRunningCondition),
						"the event must not be applied")
					succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
					if !tc.outcome {
						assert.Nil(t, succeeded, "the event must not be applied")
						return
					}
					require.NotNil(t, succeeded)
					assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeeded.Status, "the recorded outcome must stand")
					assert.Equal(t, string(sparkv1beta2.CompletedState), succeeded.Reason)
				})
			}
		})
	}
}

// TestSparkApplicationEventHandlers_WriteRetries covers how the update and
// delete handlers' status writes fail and are retried. A conflict is retried
// on a fresh copy of the job, which keeps what another writer wrote meanwhile
// and is checked again before the write; a transient failure is retried;
// nothing else is; and the retries stop after retry.DefaultBackoff's attempts.
// A job deleted before the write is the ordinary end of a job, not an error.
func TestSparkApplicationEventHandlers_WriteRetries(t *testing.T) {
	// recordedBy reports whether the handler's own record of the event, for a
	// RUNNING application, is on the job.
	recordedBy := map[string]func(got *v2pb.SparkJob) bool{
		watcherTestUpdate.name: func(got *v2pb.SparkJob) bool {
			running := findCondition(got.Status.StatusConditions, constants.SparkAppRunningCondition)
			return running != nil && running.Status == apipb.CONDITION_STATUS_TRUE
		},
		watcherTestDelete.name: func(got *v2pb.SparkJob) bool {
			succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
			return succeeded != nil && succeeded.Reason == reasonSparkApplicationDeleted
		},
	}
	// newFixture stores a scheduled job and answers the handler's status
	// writes, counted in attempts, with answer(attempt): an error fails the
	// attempt, and nil lets it through to the fake API server.
	newFixture := func(t *testing.T, answer func(ctx context.Context, c client.Client, attempt int) error) (*watcherFixture, *int) {
		t.Helper()
		attempts := new(int)
		f := newWatcherFixture(t, &interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				*attempts++
				if err := answer(ctx, c, *attempts); err != nil {
					return err
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}, scheduledWatcherTestSparkJob())
		f.projectForReal()
		return f, attempts
	}
	local := func() *sparkv1beta2.SparkApplication {
		return labeledSparkApplication(sparkv1beta2.RunningState, watcherTestJobUID)
	}

	for _, handler := range []watcherTestHandler{watcherTestUpdate, watcherTestDelete} {
		recorded := recordedBy[handler.name]
		// freezes is whether a write that records the event is followed by the
		// freeze, as only the delete handler's is.
		freezes := handler.name == watcherTestDelete.name

		t.Run(handler.name, func(t *testing.T) {
			t.Run("a conflict is retried on a fresh copy, which keeps what was written meanwhile", func(t *testing.T) {
				f, attempts := newFixture(t, func(ctx context.Context, c client.Client, attempt int) error {
					if attempt == 1 {
						// The controller records the launch between the handler's
						// read and its write, so the write is made against a stale
						// copy and conflicts.
						writeStoredCondition(t, ctx, c, watcherTestCondition(
							constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE, reasonSparkApplicationCreated))
					}
					return nil
				})

				handler.invoke(f.r, local())

				assert.Equal(t, 2, *attempts, "the conflicting write and its retry")
				got := getWatcherTestSparkJob(t, f.r)
				assert.True(t, recorded(got), "the retry must record the event")
				launched := findCondition(got.Status.StatusConditions, constants.LaunchedCondition)
				require.NotNil(t, launched, "the retry must keep what was written meanwhile")
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
				assert.Equal(t, freezes, apiutils.IsImmutable(got))
				f.assertErrorLogged(t, false)
			})

			t.Run("a conflict whose retry finds an outcome recorded writes nothing more", func(t *testing.T) {
				f, attempts := newFixture(t, func(ctx context.Context, c client.Client, attempt int) error {
					if attempt == 1 {
						// The update event that reports the application's
						// completion is written between this handler's read and
						// its write.
						writeStoredCondition(t, ctx, c, watcherTestCondition(
							constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState)))
					}
					return nil
				})

				handler.invoke(f.r, local())

				assert.Equal(t, 1, *attempts, "only the conflicting write")
				assert.Equal(t, 1, *f.writes, "no further write, the freeze included")
				got := getWatcherTestSparkJob(t, f.r)
				succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
				require.NotNil(t, succeeded)
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, succeeded.Status, "the recorded outcome must stand")
				assert.Equal(t, string(sparkv1beta2.CompletedState), succeeded.Reason)
				assert.False(t, recorded(got))
				assert.Zero(t, f.logs.FilterMessageSnippet("recorded").Len(), "no log may claim a record that was not written")
				f.assertErrorLogged(t, false)
			})

			t.Run("a transient failure is retried", func(t *testing.T) {
				f, attempts := newFixture(t, func(_ context.Context, _ client.Client, attempt int) error {
					if attempt == 1 {
						return unavailableError()
					}
					return nil
				})

				handler.invoke(f.r, local())

				assert.Equal(t, 2, *attempts)
				got := getWatcherTestSparkJob(t, f.r)
				assert.True(t, recorded(got))
				assert.Equal(t, freezes, apiutils.IsImmutable(got))
				f.assertErrorLogged(t, false)
			})

			t.Run("a failure that is neither a conflict nor transient is not retried", func(t *testing.T) {
				f, attempts := newFixture(t, func(context.Context, client.Client, int) error { return forbiddenError() })

				handler.invoke(f.r, local())

				assert.Equal(t, 1, *attempts)
				assert.Equal(t, 1, *f.writes, "nothing was recorded, so nothing is frozen")
				assert.False(t, recorded(getWatcherTestSparkJob(t, f.r)))
				f.assertErrorLogged(t, true)
			})

			t.Run("conflicts that never clear give up after retry.DefaultBackoff's attempts", func(t *testing.T) {
				f, attempts := newFixture(t, func(context.Context, client.Client, int) error { return conflictError() })

				handler.invoke(f.r, local())

				assert.Equal(t, retry.DefaultBackoff.Steps, *attempts)
				assert.Equal(t, retry.DefaultBackoff.Steps, *f.writes, "nothing was recorded, so nothing is frozen")
				assert.False(t, recorded(getWatcherTestSparkJob(t, f.r)))
				f.assertErrorLogged(t, true)
			})

			t.Run("a job deleted before the write is not an error", func(t *testing.T) {
				reads := 0
				f := newWatcherFixture(t, &interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						reads++
						if reads == 2 {
							// The ingester removes the job between the handler's
							// first read and the read inside its write attempt.
							require.NoError(t, c.Delete(ctx, readStoredSparkJob(t, ctx, c)))
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}, scheduledWatcherTestSparkJob())
				f.projectForReal()

				handler.invoke(f.r, local())

				assert.Equal(t, 2, reads)
				assert.Equal(t, 0, *f.writes)
				f.assertErrorLogged(t, false)
				assert.Equal(t, 1, f.logs.FilterMessage("global spark job no longer exists, ignoring event").Len())
			})
		})
	}
}

// TestSparkApplicationEventHandler_BadInput feeds the handler everything that
// can reach it when the cluster, the informer or the mapper is not in the
// state the happy path assumes. None of it may panic or write, and what is
// logged as an error says what went wrong.
func TestSparkApplicationEventHandler_BadInput(t *testing.T) {
	tests := []struct {
		name string
		// storeJob is false for the case where the global job is already gone.
		storeJob  bool
		object    func() interface{}
		setupMock func(m *typesmocks.MockMapper, obj interface{})
		// failFirstRead fails the handler's read of the job with a transient
		// error.
		failFirstRead bool
		wantErrorLog  bool
	}{
		{
			name:     "global spark job no longer exists",
			storeJob: false,
			object:   func() interface{} { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
		},
		{
			name:     "spark application has no project label",
			storeJob: true,
			object: func() interface{} {
				local := watcherTestSparkApplication(sparkv1beta2.RunningState)
				delete(local.Labels, constants.ProjectNameLabelKey)
				return local
			},
			wantErrorLog: true,
		},
		{
			name:     "event object is not a spark application",
			storeJob: true,
			object:   func() interface{} { return "not-a-spark-application" },
		},
		{
			name:          "reading the global spark job fails",
			storeJob:      true,
			object:        func() interface{} { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			failFirstRead: true,
			wantErrorLog:  true,
		},
		{
			name:     "mapper fails",
			storeJob: true,
			object:   func() interface{} { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			setupMock: func(m *typesmocks.MockMapper, obj interface{}) {
				m.EXPECT().MapLocalJobStatusToGlobal(obj).Return(nil, errors.New("mapping failed")).Times(1)
			},
			wantErrorLog: true,
		},
		{
			name:     "mapper returns no status",
			storeJob: true,
			object:   func() interface{} { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			setupMock: func(m *typesmocks.MockMapper, obj interface{}) {
				m.EXPECT().MapLocalJobStatusToGlobal(obj).Return(nil, nil).Times(1)
			},
			wantErrorLog: true,
		},
		{
			name:     "mapper returns a status with no spark arm",
			storeJob: true,
			object:   func() interface{} { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			setupMock: func(m *typesmocks.MockMapper, obj interface{}) {
				m.EXPECT().MapLocalJobStatusToGlobal(obj).Return(&matypes.JobStatus{}, nil).Times(1)
			},
			wantErrorLog: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var objects []client.Object
			if tc.storeJob {
				objects = append(objects, watcherTestSparkJob(
					watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, "")))
			}
			var hooks *interceptor.Funcs
			if tc.failFirstRead {
				hooks = failingFirstRead()
			}
			f := newWatcherFixture(t, hooks, objects...)

			obj := tc.object()
			if tc.setupMock != nil {
				tc.setupMock(f.mapper, obj)
			}

			assert.NotPanics(t, func() { f.r.sparkApplicationAddEventHandler(obj) })
			assert.Equal(t, 0, *f.writes)
			f.assertErrorLogged(t, tc.wantErrorLog)

			if tc.storeJob {
				assert.Len(t, getWatcherTestSparkJob(t, f.r).Status.StatusConditions, 1)
			}
		})
	}
}

// TestSparkApplicationDeleteEventHandler_ExternalDelete covers deletions the
// control plane did not ask for. What they record depends on the state the
// application was last seen in. A terminal state is the run's real outcome:
// the update event that carried it may never have been written, and the
// deletion is the last chance to record it. Any other state means the
// application was removed before it finished, which is recorded as a failure
// without a Killed condition -- Killed=TRUE is what a termination the user
// asked for records, and Uniflow takes it to mean "do not retry". A
// tombstone's last known state is read the same way. Either way the job is
// frozen, and the conditions the scheduler and the controller wrote survive.
func TestSparkApplicationDeleteEventHandler_ExternalDelete(t *testing.T) {
	const goneMessage = `the SparkApplication on cluster "member-cluster-1" was deleted before it reported an outcome`
	states := []struct {
		name        string
		local       func() *sparkv1beta2.SparkApplication
		wantStatus  apipb.ConditionStatus
		wantReason  string
		wantMessage string
		wantID      string
	}{
		{
			name: "completed",
			local: func() *sparkv1beta2.SparkApplication {
				local := watcherTestSparkApplication(sparkv1beta2.CompletedState)
				local.Status.SparkApplicationID = "spark-application-1"
				return local
			},
			wantStatus: apipb.CONDITION_STATUS_TRUE,
			wantReason: string(sparkv1beta2.CompletedState),
			wantID:     "spark-application-1",
		},
		{
			name: "failed",
			local: func() *sparkv1beta2.SparkApplication {
				local := watcherTestSparkApplication(sparkv1beta2.FailedState)
				local.Status.AppState.ErrorMessage = "driver container failed with exit code 1"
				return local
			},
			wantStatus:  apipb.CONDITION_STATUS_FALSE,
			wantReason:  string(sparkv1beta2.FailedState),
			wantMessage: "driver container failed with exit code 1",
		},
		{
			name: "failed to submit",
			local: func() *sparkv1beta2.SparkApplication {
				local := watcherTestSparkApplication(sparkv1beta2.FailedSubmissionState)
				local.Status.AppState.ErrorMessage = "failed to run spark-submit: exit status 1"
				return local
			},
			wantStatus:  apipb.CONDITION_STATUS_FALSE,
			wantReason:  string(sparkv1beta2.FailedSubmissionState),
			wantMessage: "failed to run spark-submit: exit status 1",
		},
		{
			name:        "running",
			local:       func() *sparkv1beta2.SparkApplication { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			wantStatus:  apipb.CONDITION_STATUS_FALSE,
			wantReason:  reasonSparkApplicationDeleted,
			wantMessage: goneMessage + "; last observed state: RUNNING",
		},
		{
			name:        "failing, which the operator could still have moved on to a rerun",
			local:       func() *sparkv1beta2.SparkApplication { return watcherTestSparkApplication(sparkv1beta2.FailingState) },
			wantStatus:  apipb.CONDITION_STATUS_FALSE,
			wantReason:  reasonSparkApplicationDeleted,
			wantMessage: goneMessage + "; last observed state: FAILING",
		},
		{
			name:        "never reported a state",
			local:       func() *sparkv1beta2.SparkApplication { return watcherTestSparkApplication(sparkv1beta2.NewState) },
			wantStatus:  apipb.CONDITION_STATUS_FALSE,
			wantReason:  reasonSparkApplicationDeleted,
			wantMessage: goneMessage,
		},
	}
	events := []struct {
		name   string
		object func(local *sparkv1beta2.SparkApplication) interface{}
	}{
		{
			name:   "delete event",
			object: func(local *sparkv1beta2.SparkApplication) interface{} { return local },
		},
		{
			name: "tombstone delete event",
			object: func(local *sparkv1beta2.SparkApplication) interface{} {
				return cache.DeletedFinalStateUnknown{
					Key: k8sengine.SparkLocalNamespace + "/" + sparkJobName,
					Obj: local,
				}
			},
		},
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			for _, event := range events {
				t.Run(event.name, func(t *testing.T) {
					f := newWatcherFixture(t, nil, runningWatcherTestSparkJob())
					f.projectForReal()

					f.r.sparkApplicationDeleteEventHandler(event.object(state.local()))

					// One status write for the record, one plain write for the
					// immutable annotation -- they cannot be a single call.
					assert.Equal(t, 2, *f.writes)

					got := getWatcherTestSparkJob(t, f.r)
					succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
					require.NotNil(t, succeeded)
					assert.Equal(t, state.wantStatus, succeeded.Status)
					assert.Equal(t, state.wantReason, succeeded.Reason)
					assert.Equal(t, state.wantMessage, succeeded.Message)
					assert.Equal(t, state.wantID, got.Status.ApplicationId)
					assert.Nil(t, findCondition(got.Status.StatusConditions, constants.KilledCondition),
						"only a termination the user asked for records Killed")
					assert.True(t, apiutils.IsImmutable(got), "a job whose outcome is recorded is frozen")

					for _, conditionType := range []string{
						constants.EnqueuedCondition,
						constants.ScheduledCondition,
						constants.LaunchedCondition,
						constants.SparkAppRunningCondition,
					} {
						assert.NotNil(t, findCondition(got.Status.StatusConditions, conditionType),
							"the %s condition should survive the record", conditionType)
					}
					f.assertErrorLogged(t, false)
				})
			}
		})
	}
}

// TestSparkApplicationDeleteEventHandler_FreezeRemovesTheCleanupFinalizer
// covers the watch's path to the freeze, which takes the controller's cleanup
// finalizer off in the same write: the delete handler records the outcome and
// freezes the job in two writes, as without the finalizer, and the frozen job
// no longer carries it. A finalizer of somebody else's stays.
func TestSparkApplicationDeleteEventHandler_FreezeRemovesTheCleanupFinalizer(t *testing.T) {
	job := runningWatcherTestSparkJob()
	job.Finalizers = []string{sparkApplicationCleanupFinalizer, watcherTestFinalizer}
	f := newWatcherFixture(t, nil, job)
	f.projectForReal()

	f.r.sparkApplicationDeleteEventHandler(watcherTestSparkApplication(sparkv1beta2.RunningState))

	assert.Equal(t, 2, *f.writes, "the finalizer comes off in the write that freezes the job")
	got := getWatcherTestSparkJob(t, f.r)
	succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
	require.NotNil(t, succeeded)
	assert.Equal(t, reasonSparkApplicationDeleted, succeeded.Reason)
	assert.True(t, apiutils.IsImmutable(got))
	assert.Equal(t, []string{watcherTestFinalizer}, got.Finalizers)
	f.assertErrorLogged(t, false)
}

// TestSparkApplicationDeleteEventHandler_UnmappableStatus covers a deleted
// application whose status the mapper cannot project. Its last state cannot
// be read as an outcome then, so the deletion is recorded as a disappearance,
// and the mapper's failure is logged with what went wrong.
func TestSparkApplicationDeleteEventHandler_UnmappableStatus(t *testing.T) {
	tests := []struct {
		name      string
		jobStatus *matypes.JobStatus
		err       error
	}{
		{name: "mapper fails", err: errors.New("mapping failed")},
		{name: "mapper returns no status"},
		{name: "mapper returns a status with no spark arm", jobStatus: &matypes.JobStatus{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			local := watcherTestSparkApplication(sparkv1beta2.CompletedState)
			f := newWatcherFixture(t, nil, launchedWatcherTestSparkJob())
			f.mapper.EXPECT().MapLocalJobStatusToGlobal(local).Return(tc.jobStatus, tc.err).Times(1)

			f.r.sparkApplicationDeleteEventHandler(local)

			assert.Equal(t, 2, *f.writes, "the record and the freeze")
			got := getWatcherTestSparkJob(t, f.r)
			succeeded := findCondition(got.Status.StatusConditions, constants.SucceededCondition)
			require.NotNil(t, succeeded)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, succeeded.Status)
			assert.Equal(t, reasonSparkApplicationDeleted, succeeded.Reason)
			assert.Contains(t, succeeded.Message, "last observed state: COMPLETED")
			assert.True(t, apiutils.IsImmutable(got))
			f.assertErrorLogged(t, true)
		})
	}
}

// TestSparkApplicationDeleteEventHandler_BadInput feeds the delete handler the
// events it cannot tie to a job it may write: an application without a project
// label, a job that is already gone, and a job that cannot be read. None of
// them may panic, consult the mapper or write, and only the job being gone is
// not an error: it is how every job ends.
func TestSparkApplicationDeleteEventHandler_BadInput(t *testing.T) {
	tests := []struct {
		name string
		// storeJob is false for the case where the global job is already gone.
		storeJob bool
		local    func() *sparkv1beta2.SparkApplication
		// failFirstRead fails the handler's read of the job with a transient
		// error.
		failFirstRead bool
		wantErrorLog  bool
	}{
		{
			name:     "spark application has no project label",
			storeJob: true,
			local: func() *sparkv1beta2.SparkApplication {
				local := watcherTestSparkApplication(sparkv1beta2.RunningState)
				delete(local.Labels, constants.ProjectNameLabelKey)
				return local
			},
			wantErrorLog: true,
		},
		{
			name:  "global spark job no longer exists",
			local: func() *sparkv1beta2.SparkApplication { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
		},
		{
			name:          "reading the global spark job fails",
			storeJob:      true,
			local:         func() *sparkv1beta2.SparkApplication { return watcherTestSparkApplication(sparkv1beta2.RunningState) },
			failFirstRead: true,
			wantErrorLog:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var objects []client.Object
			if tc.storeJob {
				objects = append(objects, launchedWatcherTestSparkJob())
			}
			var hooks *interceptor.Funcs
			if tc.failFirstRead {
				hooks = failingFirstRead()
			}
			f := newWatcherFixture(t, hooks, objects...)

			assert.NotPanics(t, func() { f.r.sparkApplicationDeleteEventHandler(tc.local()) })
			assert.Equal(t, 0, *f.writes)
			f.assertErrorLogged(t, tc.wantErrorLog)

			if tc.storeJob {
				got := getWatcherTestSparkJob(t, f.r)
				assert.Nil(t, findCondition(got.Status.StatusConditions, constants.SucceededCondition))
				assert.False(t, apiutils.IsImmutable(got))
			}
		})
	}
}

// TestSparkApplicationDeleteEventHandler_NoWrite covers the deletions that are
// not news: garbage collection after the outcome is already recorded, a job
// the control plane is tearing down, and a tombstone that lost its object.
func TestSparkApplicationDeleteEventHandler_NoWrite(t *testing.T) {
	tests := []struct {
		name   string
		job    func() *v2pb.SparkJob
		object func(local *sparkv1beta2.SparkApplication) interface{}
	}{
		{
			name: "application cleaned up after it completed",
			job: func() *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState)))
			},
		},
		{
			name: "application cleaned up after it failed",
			job: func() *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE, string(sparkv1beta2.FailedState)))
			},
		},
		{
			name: "application deleted by handleTermination",
			job: func() *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(
					constants.KilledCondition, apipb.CONDITION_STATUS_TRUE, constants.SparkAppKilled))
			},
		},
		{
			name: "termination was requested",
			job: func() *v2pb.SparkJob {
				job := watcherTestSparkJob(watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, ""))
				job.Spec.Termination = &v2pb.TerminationSpec{Type: v2pb.TERMINATION_TYPE_SUCCEEDED}
				return job
			},
		},
		{
			name: "tombstone holds an unexpected object",
			job: func() *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, ""))
			},
			object: func(_ *sparkv1beta2.SparkApplication) interface{} {
				return cache.DeletedFinalStateUnknown{Key: "default/" + sparkJobName, Obj: &v2pb.SparkJob{}}
			},
		},
		{
			name: "event object is not a spark application",
			job: func() *v2pb.SparkJob {
				return watcherTestSparkJob(watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, ""))
			},
			object: func(_ *sparkv1beta2.SparkApplication) interface{} { return 42 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			local := watcherTestSparkApplication(sparkv1beta2.RunningState)
			stored := tc.job()
			conditionCount := len(stored.Status.StatusConditions)
			r, _, writes := newWatcherTestReconciler(t, stored)

			object := interface{}(local)
			if tc.object != nil {
				object = tc.object(local)
			}

			assert.NotPanics(t, func() { r.sparkApplicationDeleteEventHandler(object) })

			assert.Equal(t, 0, *writes, "handler should not write the global spark job")

			got := getWatcherTestSparkJob(t, r)
			assert.Len(t, got.Status.StatusConditions, conditionCount)
			assert.False(t, apiutils.IsImmutable(got))
		})
	}
}

// TestSparkJobStatusUpdateSkipReason covers the reasons the watch and the
// controller's probe share for leaving a job alone. Either sign of a deletion
// counts: the deletion timestamp, or the deleting annotation the API server's
// delete path sets without one.
func TestSparkJobStatusUpdateSkipReason(t *testing.T) {
	tests := []struct {
		name string
		job  func() *v2pb.SparkJob
		want string
	}{
		{
			name: "nothing stands in the way",
			job:  func() *v2pb.SparkJob { return watcherTestSparkJob() },
			want: "",
		},
		{
			name: "immutable",
			job: func() *v2pb.SparkJob {
				job := watcherTestSparkJob()
				apiutils.MarkImmutable(job)
				return job
			},
			want: "job is immutable",
		},
		{
			name: "deletion timestamp set",
			job: func() *v2pb.SparkJob {
				job := watcherTestSparkJob()
				now := metav1.Now()
				job.DeletionTimestamp = &now
				return job
			},
			want: "job is being deleted",
		},
		{
			name: "deleting annotation set",
			job: func() *v2pb.SparkJob {
				job := watcherTestSparkJob()
				job.Annotations = map[string]string{api.DeletingAnnotation: "true"}
				return job
			},
			want: "job is being deleted",
		},
		{
			name: "termination requested",
			job: func() *v2pb.SparkJob {
				job := watcherTestSparkJob()
				job.Spec.Termination = &v2pb.TerminationSpec{Type: v2pb.TERMINATION_TYPE_SUCCEEDED}
				return job
			},
			want: "termination was requested",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sparkJobStatusUpdateSkipReason(tc.job()))
		})
	}
}

// TestIsSparkJobOutcomeRecorded covers how an outcome is read off the
// conditions: a Succeeded condition resolved either way, or a kill. A
// Succeeded condition still UNKNOWN is an application in flight.
func TestIsSparkJobOutcomeRecorded(t *testing.T) {
	succeeded := func(status apipb.ConditionStatus) *apipb.Condition {
		return watcherTestCondition(constants.SucceededCondition, status, "")
	}
	killed := func(status apipb.ConditionStatus) *apipb.Condition {
		return watcherTestCondition(constants.KilledCondition, status, constants.SparkAppKilled)
	}
	tests := []struct {
		name       string
		conditions []*apipb.Condition
		want       bool
	}{
		{name: "no conditions", want: false},
		{
			name:       "only the scheduler's conditions",
			conditions: []*apipb.Condition{watcherTestCondition(constants.EnqueuedCondition, apipb.CONDITION_STATUS_TRUE, "")},
			want:       false,
		},
		{name: "succeeded unknown", conditions: []*apipb.Condition{succeeded(apipb.CONDITION_STATUS_UNKNOWN)}, want: false},
		{name: "succeeded true", conditions: []*apipb.Condition{succeeded(apipb.CONDITION_STATUS_TRUE)}, want: true},
		{name: "succeeded false", conditions: []*apipb.Condition{succeeded(apipb.CONDITION_STATUS_FALSE)}, want: true},
		{name: "killed", conditions: []*apipb.Condition{killed(apipb.CONDITION_STATUS_TRUE)}, want: true},
		{
			name:       "killed while succeeded is still unknown",
			conditions: []*apipb.Condition{succeeded(apipb.CONDITION_STATUS_UNKNOWN), killed(apipb.CONDITION_STATUS_TRUE)},
			want:       true,
		},
		{
			name:       "a kill that has not happened",
			conditions: []*apipb.Condition{succeeded(apipb.CONDITION_STATUS_UNKNOWN), killed(apipb.CONDITION_STATUS_FALSE)},
			want:       false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSparkJobOutcomeRecorded(watcherTestSparkJob(tc.conditions...)))
		})
	}
}

// TestIsRetriableSparkJobWriteError covers which failed writes the watch and
// the freeze try again: a conflict, which the next attempt's fresh read
// resolves, and a transient failure, which the next attempt may not hit.
// Nothing else is retried, since another attempt would fail the same way.
func TestIsRetriableSparkJobWriteError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "conflict", err: conflictError(), want: true},
		{name: "wrapped conflict", err: fmt.Errorf("could not update the spark job: %w", conflictError()), want: true},
		{name: "service unavailable", err: unavailableError(), want: true},
		{
			name: "etcd timeout, as the API server reports it",
			err:  apiErrors.NewInternalError(errors.New("etcdserver: request timed out")),
			want: true,
		},
		{name: "not found", err: apiErrors.NewNotFound(sparkJobsResource, sparkJobName), want: false},
		{name: "forbidden", err: forbiddenError(), want: false},
		{name: "bad request", err: apiErrors.NewBadRequest("status.statusConditions: Invalid value"), want: false},
		{name: "no error", err: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetriableSparkJobWriteError(tc.err))
		})
	}
}

// TestMarkSparkJobImmutableIfOutcomeRecorded_Retries covers how the freeze,
// which the controller shares, handles a failing write: a conflict or a
// transient failure is retried, anything else is returned at once, and the
// retries stop after retry.DefaultBackoff's attempts.
func TestMarkSparkJobImmutableIfOutcomeRecorded_Retries(t *testing.T) {
	tests := []struct {
		name string
		// fail answers the given attempt, counting from 1, with an error, or with
		// nil to let it through.
		fail         func(attempt int) error
		wantAttempts int
		wantFrozen   bool
		// wantErr checks the returned error; nil when none is expected.
		wantErr func(error) bool
	}{
		{
			name: "a transient failure is retried",
			fail: func(attempt int) error {
				if attempt == 1 {
					return unavailableError()
				}
				return nil
			},
			wantAttempts: 2,
			wantFrozen:   true,
		},
		{
			name: "a conflict is retried",
			fail: func(attempt int) error {
				if attempt == 1 {
					return conflictError()
				}
				return nil
			},
			wantAttempts: 2,
			wantFrozen:   true,
		},
		{
			name:         "a failure that is neither is returned at once",
			fail:         func(int) error { return forbiddenError() },
			wantAttempts: 1,
			wantErr:      apiErrors.IsForbidden,
		},
		{
			name:         "a failure that never clears gives up after retry.DefaultBackoff's attempts",
			fail:         func(int) error { return unavailableError() },
			wantAttempts: retry.DefaultBackoff.Steps,
			wantErr:      apiErrors.IsServiceUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			f := newWatcherFixture(t, &interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					attempts++
					if err := tc.fail(attempts); err != nil {
						return err
					}
					return c.Update(ctx, obj, opts...)
				},
			}, watcherTestSparkJob(watcherTestCondition(
				constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, string(sparkv1beta2.CompletedState))))

			err := f.r.markSparkJobImmutableIfOutcomeRecorded(context.Background(), getWatcherTestSparkJob(t, f.r))

			assert.Equal(t, tc.wantAttempts, attempts)
			assert.Equal(t, tc.wantFrozen, apiutils.IsImmutable(getWatcherTestSparkJob(t, f.r)))
			if tc.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, tc.wantErr(err), "the last attempt's error should be returned, wrapped: %v", err)
		})
	}
}
