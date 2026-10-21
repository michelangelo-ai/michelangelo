package job

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const rayJobUID = types.UID("rj-uid")

var (
	dispatchEpoch = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rayJobKey     = types.NamespacedName{Name: rayJobName, Namespace: testNamespace}

	rayJobsResource = schema.GroupResource{Group: "ray.io", Resource: "rayjobs"}
	// errJobExists is what a create that finds a job of the same name returns.
	errJobExists = apiErrors.NewAlreadyExists(rayJobsResource, rayJobName)
	// ownJobOwner is the owner that the job's compute-side RayJob records.
	ownJobOwner = jobsclient.ComputeOwner{Owner: testNamespace + "/" + rayJobName, UID: string(rayJobUID)}

	// dispatchTargets are the targets the owner check runs on. The RayJob
	// reconciler reads no scheduler backend, and either backend may dispatch
	// to a cluster that Kueue manages or to one it does not.
	dispatchTargets = []struct {
		name    string
		cluster *v2pb.Cluster
	}{
		{name: "plain target", cluster: &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: assignedCluster}}},
		{name: "Kueue-managed target", cluster: &v2pb.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: assignedCluster},
			Spec:       v2pb.ClusterSpec{SchedulerType: v2pb.SCHEDULER_TYPE_KUEUE},
		}},
	}
)

// dispatchHarness runs the RayJob reconciler's create against a fake API
// server under a fake clock, counting its status writes.
type dispatchHarness struct {
	t            *testing.T
	ctx          context.Context
	server       client.WithWatch
	fed          *clientmocks.MockFederatedClient
	r            *Reconciler
	now          time.Time
	statusWrites int
}

// newDispatchHarness serves a RayJob, with the given Launched condition or
// none, on a Ready RayCluster assigned to a registered cluster.
func newDispatchHarness(t *testing.T, launched *apipb.Condition) *dispatchHarness {
	t.Helper()
	jobsclient.RegisterDispatchMetrics()
	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, v2pb.AddToScheme(scheme))

	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{Name: rayJobName, Namespace: testNamespace, Generation: 1, UID: rayJobUID},
		Spec: v2pb.RayJobSpec{
			Cluster:    &apipb.ResourceIdentifier{Name: testClusterName, Namespace: testNamespace},
			Entrypoint: "echo Hello World",
		},
	}
	if launched != nil {
		rayJob.Status.StatusConditions = []*apipb.Condition{launched}
	}
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace, Generation: 1},
		Status: v2pb.RayClusterStatus{
			State:      v2pb.RAY_CLUSTER_STATE_READY,
			Assignment: &v2pb.AssignmentInfo{Cluster: assignedCluster},
		},
	}

	h := &dispatchHarness{t: t, ctx: context.Background(), now: dispatchEpoch}
	h.server = fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(rayJob, rayCluster).WithStatusSubresource(rayJob, rayCluster).Build()
	counted := interceptor.NewClient(h.server, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			h.statusWrites++
			return c.SubResource(subResource).Update(ctx, obj, opts...)
		},
	})
	h.fed = clientmocks.NewMockFederatedClient(gomock.NewController(t))
	cache := newMockClusterCache()
	cache.addCluster(assignedCluster, &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: assignedCluster}})
	h.r = &Reconciler{
		Client:          counted,
		federatedClient: h.fed,
		clusterCache:    cache,
		now:             func() time.Time { return h.now },
	}
	return h
}

func (h *dispatchHarness) reconcile() ctrl.Result {
	h.t.Helper()
	res, err := h.r.Reconcile(h.ctx, ctrl.Request{NamespacedName: rayJobKey})
	require.NoError(h.t, err)
	return res
}

func (h *dispatchHarness) get() *v2pb.RayJob {
	h.t.Helper()
	rayJob := &v2pb.RayJob{}
	require.NoError(h.t, h.server.Get(h.ctx, rayJobKey, rayJob))
	return rayJob
}

// setTarget registers cluster as the RayJob's assigned target.
func (h *dispatchHarness) setTarget(cluster *v2pb.Cluster) {
	h.r.clusterCache.(*mockClusterCache).addCluster(cluster.GetName(), cluster)
}

// state returns the dispatch record in the stored Launched condition.
func (h *dispatchHarness) state() jobsutils.DispatchState {
	h.t.Helper()
	state, err := jobsutils.DispatchStateFromCondition(jobsutils.FindCondition(h.get().Status.StatusConditions, constants.LaunchedCondition))
	require.NoError(h.t, err)
	return state
}

func createRetries(t *testing.T, class jobsclient.DispatchClass) float64 {
	t.Helper()
	families, err := metrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "michelangelo_jobs_create_retries_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["job_type"] == dispatchMetricsJobType && labels["class"] == class.String() {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestCreateRayJobRetriesATransientFailureInPlace(t *testing.T) {
	h := newDispatchHarness(t, nil)
	before := createRetries(t, jobsclient.DispatchRejected)
	gomock.InOrder(
		// Kueue's RayJob webhook fails closed while Kueue restarts.
		h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			apiErrors.NewInternalError(errors.New(`failed calling webhook "mrayjob.kb.io": connection refused`))),
		h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)

	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
	rayJob := h.get()
	assert.NotEqual(t, v2pb.RAY_JOB_STATE_FAILED, rayJob.Status.State)
	launched := jobsutils.FindCondition(rayJob.Status.StatusConditions, constants.LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
	assert.Equal(t, jobsutils.DispatchState{
		WindowStart: dispatchEpoch, Attempts: 1, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
	}, h.state())
	assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchRejected))
	writes := h.statusWrites

	// Gate 1: the reconcile triggered by that write neither dials nor writes.
	h.now = dispatchEpoch.Add(2 * time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: 3 * time.Second}, h.reconcile())
	assert.Equal(t, writes, h.statusWrites)

	// The retry launches the job.
	h.now = dispatchEpoch.Add(5 * time.Second)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	rayJob = h.get()
	assert.Equal(t, v2pb.RAY_JOB_STATE_INITIALIZING, rayJob.Status.State)
	launched = jobsutils.FindCondition(rayJob.Status.StatusConditions, constants.LaunchedCondition)
	require.NotNil(t, launched)
	assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
	assert.Nil(t, launched.Metadata, "a launch clears the dispatch record")
}

func TestCreateRayJobFailsAtTheWindowEnd(t *testing.T) {
	metadata, err := jobsutils.DispatchState{
		WindowStart: dispatchEpoch.Add(-jobsutils.DispatchWindow), Attempts: 6, NextAttemptAt: dispatchEpoch,
	}.Metadata()
	require.NoError(t, err)
	h := newDispatchHarness(t, &apipb.Condition{
		Type: constants.LaunchedCondition, Status: apipb.CONDITION_STATUS_FALSE, Metadata: metadata,
	})
	timeout := apiErrors.NewTimeoutError("timed out", 0)
	h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(timeout)

	assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
	rayJob := h.get()
	assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, rayJob.Status.State)
	assert.Equal(t, "failed to create ray job in 7 attempts over 10m0s: "+timeout.Error(), rayJob.Status.Message)
	state := h.state()
	assert.Equal(t, 7, state.Attempts)
	assert.True(t, state.Ambiguous)
}

func TestCreateRayJobAlreadyExistsAdoptsTheJobsOwnOrAnUnownedJob(t *testing.T) {
	owners := []struct {
		name  string
		owner jobsclient.ComputeOwner
	}{
		{name: "the job's", owner: ownJobOwner},
		{name: "no owner, as on jobs created before owners were recorded"},
	}
	for _, target := range dispatchTargets {
		for _, o := range owners {
			t.Run(target.name+": "+o.name, func(t *testing.T) {
				h := newDispatchHarness(t, nil)
				h.setTarget(target.cluster)
				gomock.InOrder(
					h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), target.cluster).Return(errJobExists),
					h.fed.EXPECT().GetJobOwner(gomock.Any(), gomock.Any(), target.cluster).Return(o.owner, nil),
				)

				assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
				rayJob := h.get()
				assert.Equal(t, v2pb.RAY_JOB_STATE_INITIALIZING, rayJob.Status.State)
				launched := jobsutils.FindCondition(rayJob.Status.StatusConditions, constants.LaunchedCondition)
				require.NotNil(t, launched)
				assert.Equal(t, apipb.CONDITION_STATUS_TRUE, launched.Status)
			})
		}
	}
}

func TestCreateRayJobAlreadyExistsFailsAtOnceOnAnotherJobsJob(t *testing.T) {
	for _, target := range dispatchTargets {
		t.Run(target.name, func(t *testing.T) {
			h := newDispatchHarness(t, nil)
			h.setTarget(target.cluster)
			// ns2/test-job maps to the same compute-side name. Any further
			// create, and any delete, is an unexpected call.
			theirs := jobsclient.ComputeOwner{Owner: "ns2/" + rayJobName, UID: "other-uid"}
			gomock.InOrder(
				h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), target.cluster).Return(errJobExists),
				h.fed.EXPECT().GetJobOwner(gomock.Any(), gomock.Any(), target.cluster).Return(theirs, nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			rayJob := h.get()
			assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, rayJob.Status.State)
			message := `failed to create ray job: an object of the same name on the compute cluster is owned by another job "ns2/test-job" (uid other-uid); ` +
				"compute-side names omit the namespace, so rename this job"
			assert.Equal(t, message, rayJob.Status.Message)
			launched := jobsutils.FindCondition(rayJob.Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
			assert.Equal(t, clusterCreationFailedReason, launched.Reason)
			assert.Equal(t, message, launched.Message)
			assert.True(t, h.state().IsZero(), "no attempt is recorded")
			assert.True(t, utils.IsImmutable(rayJob), "the failed job is finalized")

			// The finalized job is not created again.
			h.now = dispatchEpoch.Add(time.Minute)
			h.reconcile()
			assert.Equal(t, v2pb.RAY_JOB_STATE_FAILED, h.get().Status.State)
		})
	}
}

func TestCreateRayJobAlreadyExistsRetriesInPlaceWhenTheOwnerCannotBeRead(t *testing.T) {
	for _, target := range dispatchTargets {
		t.Run(target.name, func(t *testing.T) {
			h := newDispatchHarness(t, nil)
			h.setTarget(target.cluster)
			before := createRetries(t, jobsclient.DispatchAmbiguous)
			readErr := fmt.Errorf("get ray job err:%w", apiErrors.NewServiceUnavailable("unavailable"))
			gomock.InOrder(
				h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errJobExists),
				h.fed.EXPECT().GetJobOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(jobsclient.ComputeOwner{}, readErr),
				// The retry meets the job again, and checks its owner again.
				h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errJobExists),
				h.fed.EXPECT().GetJobOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(ownJobOwner, nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
			rayJob := h.get()
			assert.NotEqual(t, v2pb.RAY_JOB_STATE_FAILED, rayJob.Status.State)
			launched := jobsutils.FindCondition(rayJob.Status.StatusConditions, constants.LaunchedCondition)
			require.NotNil(t, launched)
			assert.Equal(t, apipb.CONDITION_STATUS_FALSE, launched.Status)
			assert.Equal(t, "an object of the same name exists and its owner could not be read: get ray job err:unavailable", launched.Message)
			assert.Equal(t, jobsutils.DispatchState{
				WindowStart: dispatchEpoch, Attempts: 1, Ambiguous: true, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
			}, h.state())
			assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchAmbiguous))

			h.now = dispatchEpoch.Add(5 * time.Second)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			assert.Equal(t, v2pb.RAY_JOB_STATE_INITIALIZING, h.get().Status.State)
		})
	}
}

func TestCreateRayJobAlreadyExistsRetriesTheCreateWhenTheJobIsGone(t *testing.T) {
	for _, target := range dispatchTargets {
		t.Run(target.name, func(t *testing.T) {
			h := newDispatchHarness(t, nil)
			h.setTarget(target.cluster)
			before := createRetries(t, jobsclient.DispatchRejected)
			gone := fmt.Errorf("get ray job err:%w", apiErrors.NewNotFound(rayJobsResource, rayJobName))
			gomock.InOrder(
				h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errJobExists),
				h.fed.EXPECT().GetJobOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(jobsclient.ComputeOwner{}, gone),
				h.fed.EXPECT().CreateJob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)

			assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Second}, h.reconcile())
			assert.NotEqual(t, v2pb.RAY_JOB_STATE_FAILED, h.get().Status.State)
			assert.Equal(t, jobsutils.DispatchState{
				WindowStart: dispatchEpoch, Attempts: 1, NextAttemptAt: dispatchEpoch.Add(5 * time.Second),
			}, h.state(), "a create that created nothing is not ambiguous")
			assert.Equal(t, before+1, createRetries(t, jobsclient.DispatchRejected))

			h.now = dispatchEpoch.Add(5 * time.Second)
			assert.Equal(t, ctrl.Result{RequeueAfter: requeueAfter}, h.reconcile())
			assert.Equal(t, v2pb.RAY_JOB_STATE_INITIALIZING, h.get().Status.State)
		})
	}
}
