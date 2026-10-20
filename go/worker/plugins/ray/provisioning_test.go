package ray

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/temporal"
	tmpworkflow "go.temporal.io/sdk/workflow"
	cadworkflow "go.uber.org/cadence/workflow"
	"go.uber.org/yarpc/yarpcerrors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const testBudget = 100 * time.Second

// budgetStar is the Starlark module the budget tests run: create_cluster with timeout_seconds, the provisioning
// budget.
const budgetStar = `load("@plugin", "ray")

def test_create_cluster(timeout_seconds):
    spec = {
        "metadata": {
            "name": "uf-ray-test",
            "namespace": "ma-dev-test",
        },
    }
    return ray.create_cluster(spec, timeout_seconds = timeout_seconds)
`

// budgetTest covers create_cluster's provisioning budget (provisioningBudgetVersion) on both workflow engines.
type budgetTest struct {
	suite.Suite
	service.TestSuite
	// root is the root directory of the Starlark modules the tests run.
	root string
}

func TestProvisioningBudget(t *testing.T) { suite.Run(t, new(budgetTest)) }

func (r *budgetTest) SetupSuite() {
	r.root = r.T().TempDir()
	r.Require().NoError(os.WriteFile(filepath.Join(r.root, "budget.star"), []byte(budgetStar), 0o600))
}

// engine is one workflow engine's test environment, reduced to what the budget tests use.
type engine struct {
	name string
	// onActivity registers activity and mocks it with fn, for any number of calls.
	onActivity func(activity, fn interface{})
	// now is the workflow clock.
	now func() time.Time
	// cancelAfter cancels the workflow after d of workflow time.
	cancelAfter func(d time.Duration)
	// createCluster runs create_cluster with timeout_seconds = budget and returns its result.
	createCluster func(budget time.Duration) (any, error)
	// terminalError is the error the sensors return for a FAILED or TERMINATED cluster, non-retryable on the
	// engine.
	terminalError func(ctx context.Context) error
	// assertExpectations checks the environment's mocks.
	assertExpectations func(t *testing.T)
}

// newEngines returns fresh Cadence and Temporal test environments whose workflow executions start after the
// provisioning budget was deployed.
func (r *budgetTest) newEngines() []engine {
	env := r.NewTestEnvironment(r.T(), &service.TestEnvironmentParams{
		RootDirectory: r.root,
		Plugins:       map[string]service.IPlugin{"ray": Plugin},
	})
	cad, tmp := env.Cadence.GetTestWorkflowEnvironment(), env.Temporal.GetTestWorkflowEnvironment()
	cad.OnGetVersion(provisioningBudgetChangeID, cadworkflow.DefaultVersion, provisioningBudgetVersion).
		Return(cadworkflow.Version(provisioningBudgetVersion))
	tmp.OnGetVersion(provisioningBudgetChangeID, tmpworkflow.DefaultVersion, provisioningBudgetVersion).
		Return(tmpworkflow.Version(provisioningBudgetVersion))

	return []engine{
		{
			name: "cadence",
			onActivity: func(activity, fn interface{}) {
				cad.RegisterActivity(activity)
				cad.OnActivity(activity, mock.Anything, mock.Anything).Return(fn).Maybe()
			},
			now:         cad.Now,
			cancelAfter: func(d time.Duration) { cad.RegisterDelayedCallback(cad.CancelWorkflow, d) },
			createCluster: func(budget time.Duration) (any, error) {
				env.Cadence.ExecuteFunction("/budget.star", "test_create_cluster", budgetArgs(budget), nil, nil)
				var res any
				err := env.Cadence.GetResult(&res)
				return res, err
			},
			terminalError: func(ctx context.Context) error {
				return workflow.NewCustomError(ctx, yarpcerrors.CodeInternal.String())
			},
			assertExpectations: env.Cadence.AssertExpectations,
		},
		{
			name: "temporal",
			onActivity: func(activity, fn interface{}) {
				tmp.RegisterActivity(activity)
				tmp.OnActivity(activity, mock.Anything, mock.Anything).Return(fn).Maybe()
			},
			now:         tmp.Now,
			cancelAfter: func(d time.Duration) { tmp.RegisterDelayedCallback(tmp.CancelWorkflow, d) },
			createCluster: func(budget time.Duration) (any, error) {
				env.Temporal.ExecuteFunction("/budget.star", "test_create_cluster", budgetArgs(budget), nil, nil)
				var res any
				err := env.Temporal.GetResult(&res)
				return res, err
			},
			terminalError: func(context.Context) error {
				return temporal.NewNonRetryableApplicationError(yarpcerrors.CodeInternal.String(), yarpcerrors.CodeInternal.String(), nil)
			},
			assertExpectations: env.Temporal.AssertExpectations,
		},
	}
}

func budgetArgs(budget time.Duration) starlark.Tuple {
	return starlark.Tuple{starlark.MakeInt64(int64(budget / time.Second))}
}

// phase is a state the fake cluster stays in for d; the last phase lasts forever.
type phase struct {
	state v2pb.RayClusterState
	d     time.Duration
}

// fakeCluster is a RayCluster whose state follows a timeline on the workflow clock from its creation. It
// records how the workflow terminated it.
type fakeCluster struct {
	timeline   []phase
	conditions []*apipb.Condition
	now        func() time.Time
	created    time.Time
	// terminated is the workflow's termination request, sent terminatedAfter the creation.
	terminated      *ray.TerminateClusterRequest
	terminatedAfter time.Duration
}

func (f *fakeCluster) get() *v2pb.RayCluster {
	state := f.timeline[len(f.timeline)-1].state
	elapsed := f.now().Sub(f.created)
	for _, p := range f.timeline[:len(f.timeline)-1] {
		if elapsed < p.d {
			state = p.state
			break
		}
		elapsed -= p.d
	}
	return &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "uf-ray-test", Namespace: "ma-dev-test"},
		Status: v2pb.RayClusterStatus{
			State:            state,
			JobUrl:           "http://ray-dashboard",
			StatusConditions: f.conditions,
		},
	}
}

// mock serves e's create_cluster activities from f. The sensors map states as SensorRayClusterProvisioning and
// SensorRayClusterAdmission do (activities/ray/ray_sensors_test.go pins that mapping).
func (f *fakeCluster) mock(e engine) {
	f.now = e.now
	pending := func(ctx context.Context, state v2pb.RayClusterState) error {
		return workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), state.String())
	}
	e.onActivity(ray.Activities.CreateRayCluster, func(ctx context.Context, req v2pb.CreateRayClusterRequest) (*ray.CreateRayClusterActivityResponse, error) {
		f.created = f.now()
		return &ray.CreateRayClusterActivityResponse{RayCluster: f.get(), ActivityID: "create-activity"}, nil
	})
	e.onActivity(ray.Activities.SensorRayClusterProvisioning, func(ctx context.Context, req v2pb.GetRayClusterRequest) (*ray.SensorRayClusterProvisioningResponse, error) {
		cluster := f.get()
		switch state := cluster.Status.State; state {
		case v2pb.RAY_CLUSTER_STATE_READY, v2pb.RAY_CLUSTER_STATE_SUSPENDED:
			return &ray.SensorRayClusterProvisioningResponse{
				RayCluster: cluster,
				JobURL:     cluster.Status.JobUrl,
				Ready:      state == v2pb.RAY_CLUSTER_STATE_READY,
			}, nil
		case v2pb.RAY_CLUSTER_STATE_FAILED, v2pb.RAY_CLUSTER_STATE_TERMINATED:
			return nil, e.terminalError(ctx)
		default:
			return nil, pending(ctx, state)
		}
	})
	e.onActivity(ray.Activities.SensorRayClusterAdmission, func(ctx context.Context, req v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, error) {
		cluster := f.get()
		switch state := cluster.Status.State; state {
		case v2pb.RAY_CLUSTER_STATE_SUSPENDED:
			return nil, pending(ctx, state)
		case v2pb.RAY_CLUSTER_STATE_FAILED, v2pb.RAY_CLUSTER_STATE_TERMINATED:
			return nil, e.terminalError(ctx)
		default:
			return &v2pb.GetRayClusterResponse{RayCluster: cluster}, nil
		}
	})
	e.onActivity(ray.Activities.GetRayCluster, func(ctx context.Context, req v2pb.GetRayClusterRequest) (*v2pb.GetRayClusterResponse, error) {
		return &v2pb.GetRayClusterResponse{RayCluster: f.get()}, nil
	})
	e.onActivity(ray.Activities.TerminateCluster, func(ctx context.Context, req ray.TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
		f.terminated, f.terminatedAfter = &req, f.now().Sub(f.created)
		return &v2pb.UpdateRayClusterResponse{RayCluster: f.get()}, nil
	})
}

// Conditions of a cluster stuck provisioning: the newest one, by timestamp, is the reported failure.
var provisioningConditions = []*apipb.Condition{
	{Type: utils.EnqueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "AddedToSchedulerQueue", LastUpdatedTimestamp: 1000},
	{Type: utils.LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "Launched", Message: "head pod pending: 0/3 nodes are available", LastUpdatedTimestamp: 1060},
	{Type: utils.ScheduledCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "Scheduled", LastUpdatedTimestamp: 1010},
}

const provisioningFailureMessage = "Launched: head pod pending: 0/3 nodes are available"

func (r *budgetTest) TestProvisioningBudget() {
	b := testBudget
	for _, tc := range []struct {
		name       string
		timeline   []phase
		conditions []*apipb.Condition
		// wantFailure is the task error and the termination reason; empty when the cluster becomes READY.
		wantFailure string
		// failsAfter is when the workflow terminates the cluster, within half a poll: the sensors are instant and
		// the phases whole polls, so one poll more or less is a different budget.
		failsAfter time.Duration
	}{
		{
			name:     "suspended for twice the budget, then ready",
			timeline: []phase{{v2pb.RAY_CLUSTER_STATE_SUSPENDED, 2 * b}, {state: v2pb.RAY_CLUSTER_STATE_READY}},
		},
		{
			name: "placement and provisioning within the budget around a long suspension",
			timeline: []phase{
				{v2pb.RAY_CLUSTER_STATE_INVALID, b / 5},
				{v2pb.RAY_CLUSTER_STATE_SUSPENDED, 3 * b},
				{v2pb.RAY_CLUSTER_STATE_PROVISIONING, b / 2},
				{state: v2pb.RAY_CLUSTER_STATE_READY},
			},
		},
		{
			name:        "never ready",
			timeline:    []phase{{state: v2pb.RAY_CLUSTER_STATE_PROVISIONING}},
			conditions:  provisioningConditions,
			wantFailure: provisioningFailureMessage,
			failsAfter:  b,
		},
		{
			name: "evicted after 0.6 of the budget, never ready",
			timeline: []phase{
				{v2pb.RAY_CLUSTER_STATE_PROVISIONING, b * 6 / 10},
				{v2pb.RAY_CLUSTER_STATE_SUSPENDED, 2 * b},
				{state: v2pb.RAY_CLUSTER_STATE_PROVISIONING},
			},
			conditions:  provisioningConditions,
			wantFailure: provisioningFailureMessage,
			// The eviction pauses the budget without resetting it: the cluster fails once its non-suspended time
			// reaches the budget, not at b (suspension counted) nor at 0.6b + 2b + b (budget reset).
			failsAfter: b*6/10 + 2*b + b*4/10,
		},
		{
			name:     "admission timeout while suspended",
			timeline: []phase{{v2pb.RAY_CLUSTER_STATE_SUSPENDED, b / 2}, {state: v2pb.RAY_CLUSTER_STATE_FAILED}},
			conditions: []*apipb.Condition{
				{Type: utils.EnqueuedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "AddedToSchedulerQueue", LastUpdatedTimestamp: 1000},
				{Type: utils.SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "AdmissionTimeout", Message: "waited 24h0m0s for admission", LastUpdatedTimestamp: 1050},
				{Type: "Queued", Status: apipb.CONDITION_STATUS_TRUE, Reason: "AwaitingAdmission", LastUpdatedTimestamp: 1100},
			},
			wantFailure: "AdmissionTimeout: waited 24h0m0s for admission",
			failsAfter:  b / 2,
		},
		{
			name:     "failed while provisioning",
			timeline: []phase{{v2pb.RAY_CLUSTER_STATE_PROVISIONING, b * 3 / 10}, {state: v2pb.RAY_CLUSTER_STATE_FAILED}},
			conditions: []*apipb.Condition{
				{Type: utils.SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "ClusterCreationFailed", Message: "image pull failed", LastUpdatedTimestamp: 1030},
			},
			wantFailure: "ClusterCreationFailed: image pull failed",
			failsAfter:  b * 3 / 10,
		},
	} {
		r.Run(tc.name, func() {
			for _, e := range r.newEngines() {
				r.Run(e.name, func() {
					f := &fakeCluster{timeline: tc.timeline, conditions: tc.conditions}
					f.mock(e)

					res, err := e.createCluster(b)

					e.assertExpectations(r.T())
					if tc.wantFailure == "" {
						r.Require().NoError(err)
						r.Nil(f.terminated, "a ready cluster is not terminated")
						r.Greater(e.now().Sub(f.created), b, "time suspended does not count against the budget")
						response, ok := res.(map[string]interface{})
						r.Require().True(ok, "result: %v", res)
						r.Equal("create-activity", response["activityId"])
						r.NotNil(response["rayCluster"])
						return
					}
					r.Require().Error(err)
					r.Contains(err.Error(), tc.wantFailure)
					r.Require().NotNil(f.terminated, "a cluster that does not become ready is terminated")
					r.Equal(tc.wantFailure, f.terminated.Reason)
					r.Equal(v2pb.TERMINATION_TYPE_FAILED.String(), f.terminated.Type)
					r.InDelta(tc.failsAfter.Seconds(), f.terminatedAfter.Seconds(), float64(poll)/2)
				})
			}
		})
	}
}

func (r *budgetTest) TestCanceledWhileProvisioning() {
	for _, e := range r.newEngines() {
		r.Run(e.name, func() {
			f := &fakeCluster{timeline: []phase{{state: v2pb.RAY_CLUSTER_STATE_PROVISIONING}}, conditions: provisioningConditions}
			f.mock(e)
			e.cancelAfter(testBudget / 2)

			_, err := e.createCluster(testBudget)

			r.Require().Error(err)
			r.Require().NotNil(f.terminated, "a canceled provisioning terminates the cluster")
			r.Equal("Canceled", f.terminated.Reason)
		})
	}
}

func TestLatestCondition(t *testing.T) {
	succeededFalse := &apipb.Condition{Type: utils.SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "AdmissionTimeout", LastUpdatedTimestamp: 10}
	succeededTrue := &apipb.Condition{Type: utils.SucceededCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "Succeeded", LastUpdatedTimestamp: 50}
	enqueuedFalse := &apipb.Condition{Type: utils.EnqueuedCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoReadyCluster", LastUpdatedTimestamp: 20}
	launched := &apipb.Condition{Type: utils.LaunchedCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "Launched", LastUpdatedTimestamp: 30}
	scheduled := &apipb.Condition{Type: utils.ScheduledCondition, Status: apipb.CONDITION_STATUS_TRUE, Reason: "Scheduled", LastUpdatedTimestamp: 30}
	withoutReason := &apipb.Condition{Type: utils.PendingCondition, Status: apipb.CONDITION_STATUS_UNKNOWN, LastUpdatedTimestamp: 40}

	for _, tc := range []struct {
		name       string
		conditions []*apipb.Condition
		want       *apipb.Condition
	}{
		{name: "no conditions"},
		{name: "Succeeded=False before newer conditions", conditions: []*apipb.Condition{launched, enqueuedFalse, succeededFalse}, want: succeededFalse},
		{name: "Enqueued=False before newer conditions", conditions: []*apipb.Condition{enqueuedFalse, launched}, want: enqueuedFalse},
		{name: "Succeeded=True is not a failure", conditions: []*apipb.Condition{succeededTrue, enqueuedFalse}, want: enqueuedFalse},
		{name: "else the newest", conditions: []*apipb.Condition{launched, succeededTrue, scheduled}, want: succeededTrue},
		{name: "a tie goes to the later condition", conditions: []*apipb.Condition{launched, scheduled}, want: scheduled},
		{name: "conditions without a reason are skipped", conditions: []*apipb.Condition{launched, withoutReason, nil}, want: launched},
		{name: "only conditions without a reason", conditions: []*apipb.Condition{withoutReason}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, latestCondition(tc.conditions))
		})
	}
}

func TestProvisioningFailure(t *testing.T) {
	cluster := func(conditions ...*apipb.Condition) *v2pb.RayCluster {
		return &v2pb.RayCluster{Status: v2pb.RayClusterStatus{StatusConditions: conditions}}
	}
	for _, tc := range []struct {
		name    string
		cluster *v2pb.RayCluster
		want    string
	}{
		{name: "cluster not read", want: "fallback"},
		{name: "no condition with a reason", cluster: cluster(&apipb.Condition{Type: utils.PendingCondition}), want: "fallback"},
		{
			name:    "reason and message",
			cluster: cluster(&apipb.Condition{Type: utils.SucceededCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "AdmissionTimeout", Message: "waited 24h0m0s"}),
			want:    "AdmissionTimeout: waited 24h0m0s",
		},
		{
			name:    "reason only",
			cluster: cluster(&apipb.Condition{Type: utils.EnqueuedCondition, Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoReadyCluster"}),
			want:    "NoReadyCluster",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, provisioningFailure(tc.cluster, "fallback"))
		})
	}
}

func TestActivityOptions(t *testing.T) {
	options := activityOptions(10*time.Second, time.Minute)

	require.Equal(t, time.Minute, options.ScheduleToCloseTimeout, "Temporal enforces the expiration")
	require.Equal(t, time.Minute, options.ScheduleToStartTimeout)
	require.Equal(t, service.DefaultActivityOptions.StartToCloseTimeout, options.StartToCloseTimeout)
	require.Equal(t, 10*time.Second, options.RetryPolicy.InitialInterval)
	require.Equal(t, time.Minute, options.RetryPolicy.ExpirationInterval, "Cadence enforces the expiration")
	require.Equal(t, 1.0, options.RetryPolicy.BackoffCoefficient)
	require.Equal(t, int32(8), options.RetryPolicy.MaximumAttempts)
	require.Equal(t, utils.DefaultNonRetriableErrorReasons, options.RetryPolicy.NonRetriableErrorReasons)
	require.Equal(t, utils.LongTimeout, utils.DefaultSensorRetryPolicy.ExpirationInterval, "the shared policy is copied, not changed")
	require.Zero(t, utils.DefaultSensorRetryPolicy.MaximumAttempts, "the shared policy is copied, not changed")
}

func TestMaxAttempts(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		interval, expiration time.Duration
		want                 int32
	}{
		// Attempts at 0, 10s, ..., 100s fit in the expiration.
		{name: "whole intervals", interval: 10 * time.Second, expiration: 100 * time.Second, want: 12},
		{name: "a partial interval", interval: 10 * time.Second, expiration: 105 * time.Second, want: 12},
		{name: "shorter than an interval", interval: 10 * time.Second, expiration: 3 * time.Second, want: 2},
		{name: "capped", interval: time.Nanosecond, expiration: utils.LongTimeout, want: math.MaxInt32},
		{name: "no interval", expiration: time.Minute, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, maxAttempts(tc.interval, tc.expiration))
		})
	}
}
