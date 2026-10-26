package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _now = time.Unix(1_800_000_000, 0)

// condition returns a condition that changed to the status at the given time.
// Conditions are written only when they change, so the timestamp, in Unix
// seconds as jobsutils.UpdateCondition stamps it, is when it changed.
func condition(conditionType string, status apipb.ConditionStatus, at time.Time) *apipb.Condition {
	return &apipb.Condition{Type: conditionType, Status: status, LastUpdatedTimestamp: at.Unix()}
}

func launched(ago time.Duration) *apipb.Condition {
	return condition(constants.LaunchedCondition, apipb.CONDITION_STATUS_TRUE, _now.Add(-ago))
}

func queued(status apipb.ConditionStatus, ago time.Duration) *apipb.Condition {
	return condition(constants.QueuedCondition, status, _now.Add(-ago))
}

func TestInFlightRule(t *testing.T) {
	tests := []struct {
		name        string
		state       v2pb.RayClusterState
		conditions  []*apipb.Condition
		termination v2pb.TerminationType
		deleting    bool
		want        bool
	}{
		{name: "not launched yet", want: true},
		{
			name:       "launch pending",
			conditions: []*apipb.Condition{condition(constants.LaunchedCondition, apipb.CONDITION_STATUS_UNKNOWN, _now.Add(-time.Hour))},
			want:       true,
		},
		{
			name:       "launch not done",
			conditions: []*apipb.Condition{condition(constants.LaunchedCondition, apipb.CONDITION_STATUS_FALSE, _now.Add(-time.Hour))},
			want:       true,
		},
		{name: "launched 10s ago", conditions: []*apipb.Condition{launched(10 * time.Second)}, want: true},
		{name: "launched 29s ago", conditions: []*apipb.Condition{launched(29 * time.Second)}, want: true},
		{name: "launched 30s ago", conditions: []*apipb.Condition{launched(30 * time.Second)}},
		{name: "launched an hour ago", conditions: []*apipb.Condition{launched(time.Hour)}},
		{
			name:       "held for 10s",
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_TRUE, 10*time.Second)},
			want:       true,
		},
		{
			name:       "held for 30s",
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_TRUE, 30*time.Second)},
		},
		{
			name:       "admitted 10s ago",
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_FALSE, 10*time.Second)},
			want:       true,
		},
		{
			name:       "admitted 40s ago",
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_FALSE, 40*time.Second)},
		},
		{
			// the Queued condition decides once it is True or False
			name:       "held for 10s, launched long ago",
			conditions: []*apipb.Condition{launched(time.Hour), queued(apipb.CONDITION_STATUS_TRUE, 10*time.Second)},
			want:       true,
		},
		{
			name:       "Queued Unknown, launched 10s ago",
			conditions: []*apipb.Condition{launched(10 * time.Second), queued(apipb.CONDITION_STATUS_UNKNOWN, time.Hour)},
			want:       true,
		},
		{
			name:       "Queued Unknown, launched 40s ago",
			conditions: []*apipb.Condition{launched(40 * time.Second), queued(apipb.CONDITION_STATUS_UNKNOWN, 5*time.Second)},
		},
		{
			name:       "held, stamped in the future",
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_TRUE, -time.Minute)},
			want:       true,
		},
		{
			name: "held for 10s, stamped in milliseconds",
			conditions: []*apipb.Condition{launched(time.Minute), {
				Type:                 constants.QueuedCondition,
				Status:               apipb.CONDITION_STATUS_TRUE,
				LastUpdatedTimestamp: _now.Add(-10 * time.Second).UnixMilli(),
			}},
			want: true,
		},
		{
			name: "held for 40s, stamped in milliseconds",
			conditions: []*apipb.Condition{launched(time.Minute), {
				Type:                 constants.QueuedCondition,
				Status:               apipb.CONDITION_STATUS_TRUE,
				LastUpdatedTimestamp: _now.Add(-40 * time.Second).UnixMilli(),
			}},
		},
		{
			name:       "ready, admitted 10s ago",
			state:      v2pb.RAY_CLUSTER_STATE_READY,
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_FALSE, 10*time.Second)},
			want:       true,
		},
		{
			name:       "suspended, held for 10s",
			state:      v2pb.RAY_CLUSTER_STATE_SUSPENDED,
			conditions: []*apipb.Condition{launched(time.Minute), queued(apipb.CONDITION_STATUS_TRUE, 10*time.Second)},
			want:       true,
		},
		{name: "terminating", state: v2pb.RAY_CLUSTER_STATE_TERMINATING},
		{name: "terminated", state: v2pb.RAY_CLUSTER_STATE_TERMINATED},
		{name: "failed", state: v2pb.RAY_CLUSTER_STATE_FAILED},
		{name: "termination requested", termination: v2pb.TERMINATION_TYPE_FAILED},
		{name: "termination on success requested", termination: v2pb.TERMINATION_TYPE_SUCCEEDED},
		{
			name:       "succeeded",
			conditions: []*apipb.Condition{condition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, _now)},
		},
		{
			name:       "did not succeed",
			conditions: []*apipb.Condition{condition(constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE, _now)},
		},
		{
			name:       "outcome undecided",
			conditions: []*apipb.Condition{condition(constants.SucceededCondition, apipb.CONDITION_STATUS_UNKNOWN, _now)},
			want:       true,
		},
		{name: "being deleted", deleting: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := assignedJob("job-1", "cluster-1", 2, tt.conditions...)
			job.Status.State = tt.state
			if tt.termination != v2pb.TERMINATION_TYPE_INVALID {
				job.Spec.Termination = &v2pb.TerminationSpec{Type: tt.termination}
			}
			if tt.deleting {
				job.DeletionTimestamp = &metav1.Time{Time: _now}
			}
			assert.Equal(t, tt.want, inFlight(job, _now))
		})
	}
}

func TestConditionTime(t *testing.T) {
	tests := []struct {
		name      string
		timestamp int64
		want      time.Time
	}{
		{name: "Unix seconds", timestamp: 1_800_000_000, want: time.Unix(1_800_000_000, 0)},
		{name: "Unix milliseconds", timestamp: 1_800_000_000_123, want: time.UnixMilli(1_800_000_000_123)},
		{name: "the largest seconds", timestamp: 99_999_999_999, want: time.Unix(99_999_999_999, 0)},
		{name: "the smallest milliseconds", timestamp: 100_000_000_000, want: time.UnixMilli(100_000_000_000)},
		{name: "unset", want: time.Unix(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditionTime(&apipb.Condition{LastUpdatedTimestamp: tt.timestamp})
			assert.True(t, tt.want.Equal(got), "got %v, want %v", got, tt.want)
		})
	}
}

// assignedJob returns a RayCluster of team-a with the given number of workers,
// assigned to the cluster, with the conditions.
func assignedJob(name, cluster string, workers int32, conditions ...*apipb.Condition) *v2pb.RayCluster {
	job := newJob(name, workers)
	job.Status.Assignment = &v2pb.AssignmentInfo{Cluster: cluster}
	job.Status.StatusConditions = conditions
	return job
}

func newCluster(name string, schedulerType v2pb.SchedulerType) *v2pb.Cluster {
	return &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: v2pb.ClusterSpec{SchedulerType: schedulerType}}
}

// newCache returns a fake manager cache that holds the objects, indexed as the
// module indexes the manager cache.
func newCache(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) client.Reader {
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithIndex(&v2pb.RayCluster{}, AssignedClusterIndex, assignedCluster).
		WithInterceptorFuncs(funcs).
		Build()
}

func newTestTracker(t *testing.T, mapper types.Mapper, funcs interceptor.Funcs, objs ...client.Object) *tracker {
	return newTracker(newCache(t, funcs, objs...), mapper, logr.Discard())
}

func jobNames(f InFlight) []string {
	var names []string
	for _, job := range f.Jobs {
		names = append(names, job.RayCluster.Name)
	}
	return names
}

func TestTrackerInFlight(t *testing.T) {
	ctx := context.Background()
	// a job with n workers asks for 2+4n CPUs, 8+16n Gi of memory and n GPUs
	objs := []client.Object{
		assignedJob("pending", "cluster-1", 1),
		assignedJob("held", "cluster-1", 3, launched(time.Hour), queued(apipb.CONDITION_STATUS_TRUE, 10*time.Second)),
		assignedJob("running", "cluster-1", 2, launched(time.Hour), queued(apipb.CONDITION_STATUS_FALSE, time.Hour)),
		assignedJob("finished", "cluster-1", 2, condition(constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, _now)),
		assignedJob("elsewhere", "cluster-2", 2),
		newJob("unassigned", 2),
	}

	t.Run("the jobs in flight to the cluster", func(t *testing.T) {
		got, err := newTestTracker(t, newMapper(false), interceptor.Funcs{}, objs...).inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"pending", "held"}, jobNames(got))
		for _, job := range got.Jobs {
			assert.Equal(t, "ma-team-a", job.LocalQueue)
			if job.RayCluster.Name == "pending" {
				requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), job.Ask)
			} else {
				requireAsk(t, resources("cpu", "14", "memory", "56Gi", "nvidia.com/gpu", "3"), job.Ask)
			}
		}
		requireAsk(t, resources("cpu", "20", "memory", "80Gi", "nvidia.com/gpu", "4"), got.Ask)
	})

	t.Run("on a cluster Kueue does not manage, no LocalQueue", func(t *testing.T) {
		got, err := newTestTracker(t, newMapper(false), interceptor.Funcs{}, objs...).
			inFlight(ctx, newCluster("cluster-1", v2pb.SCHEDULER_TYPE_DEFAULT), _now)
		require.NoError(t, err)
		require.Len(t, got.Jobs, 2)
		for _, job := range got.Jobs {
			assert.Empty(t, job.LocalQueue)
		}
	})

	t.Run("none in flight", func(t *testing.T) {
		got, err := newTestTracker(t, newMapper(false), interceptor.Funcs{}, objs...).
			inFlight(ctx, newCluster("cluster-3", v2pb.SCHEDULER_TYPE_KUEUE), _now)
		require.NoError(t, err)
		assert.Empty(t, got.Jobs)
		assert.NotNil(t, got.Ask)
		assert.Empty(t, got.Ask)
	})

	t.Run("a job that cannot be mapped is left out", func(t *testing.T) {
		k8sengineMapper := newMapper(false)
		mapper := typesmocks.NewMockMapper(gomock.NewController(t))
		mapper.EXPECT().MapGlobalJobClusterToLocal(gomock.Any(), gomock.Any()).DoAndReturn(
			func(obj runtime.Object, c *v2pb.Cluster) (runtime.Object, error) {
				if obj.(*v2pb.RayCluster).Name == "held" {
					return nil, errors.New("no project identity")
				}
				return k8sengineMapper.MapGlobalJobClusterToLocal(obj, c)
			}).AnyTimes()
		got, err := newTestTracker(t, mapper, interceptor.Funcs{}, objs...).inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		require.Equal(t, []string{"pending"}, jobNames(got))
		requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), got.Ask)
	})

	t.Run("the cache cannot be listed", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("cache stopped")
			},
		}, objs...)
		_, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.ErrorContains(t, err, "list the RayClusters assigned to cluster cluster-1: cache stopped")
	})
}

func TestTrackerRecordedAssignments(t *testing.T) {
	ctx := context.Background()
	otherCluster := newCluster("cluster-2", v2pb.SCHEDULER_TYPE_KUEUE)

	t.Run("counted until the cache shows it, for less than 30s", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{})
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		for _, after := range []time.Duration{0, 29 * time.Second} {
			got, err := tr.inFlight(ctx, _kueueCluster, _now.Add(after))
			require.NoError(t, err)
			require.Equal(t, []string{"job-1"}, jobNames(got))
			assert.Equal(t, "ma-team-a", got.Jobs[0].LocalQueue)
			requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), got.Ask)
		}

		got, err := tr.inFlight(ctx, otherCluster, _now)
		require.NoError(t, err)
		assert.Empty(t, got.Jobs, "recorded for another cluster")

		got, err = tr.inFlight(ctx, _kueueCluster, _now.Add(30*time.Second))
		require.NoError(t, err)
		assert.Empty(t, got.Jobs)
		assert.Empty(t, tr.recorded, "the expired assignment was kept")
	})

	t.Run("recorded again, counted from then", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{})
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now.Add(20*time.Second))
		got, err := tr.inFlight(ctx, _kueueCluster, _now.Add(40*time.Second))
		require.NoError(t, err)
		assert.Equal(t, []string{"job-1"}, jobNames(got))
		got, err = tr.inFlight(ctx, _kueueCluster, _now.Add(50*time.Second))
		require.NoError(t, err)
		assert.Empty(t, got.Jobs)
	})

	t.Run("the cache shows the assignment: counted once, from the cache", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{}, assignedJob("job-1", "cluster-1", 1))
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		got, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		assert.Equal(t, []string{"job-1"}, jobNames(got))
		requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), got.Ask)
		assert.Empty(t, tr.recorded, "the shown assignment was kept")
	})

	t.Run("the cache shows the assignment no longer in flight", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{},
			assignedJob("job-1", "cluster-1", 1, launched(time.Minute), queued(apipb.CONDITION_STATUS_FALSE, time.Minute)))
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		got, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		assert.Empty(t, got.Jobs)
		assert.Empty(t, tr.recorded)
	})

	t.Run("the cache shows the job, not yet assigned", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{}, newJob("job-1", 1))
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		got, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		assert.Equal(t, []string{"job-1"}, jobNames(got))
		assert.Len(t, tr.recorded, 1)
	})

	t.Run("the cache shows the job assigned to another cluster", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{}, assignedJob("job-1", "cluster-2", 1))
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		got, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		assert.Empty(t, got.Jobs)
		assert.Empty(t, tr.recorded)
		got, err = tr.inFlight(ctx, otherCluster, _now)
		require.NoError(t, err)
		assert.Equal(t, []string{"job-1"}, jobNames(got))
	})

	t.Run("after a failover, the persisted assignments still count", func(t *testing.T) {
		objs := []client.Object{assignedJob("persisted", "cluster-1", 1)}
		leader := newTestTracker(t, newMapper(false), interceptor.Funcs{}, objs...)
		leader.recordAssignment(newJob("unpersisted", 1), "cluster-1", _now)
		got, err := leader.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"persisted", "unpersisted"}, jobNames(got))

		// a new leader starts with no recorded assignments
		got, err = newTestTracker(t, newMapper(false), interceptor.Funcs{}, objs...).inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		assert.Equal(t, []string{"persisted"}, jobNames(got))
	})

	t.Run("the recorded job is copied", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{})
		job := newJob("job-1", 1)
		tr.recordAssignment(job, "cluster-1", _now)
		job.Spec.Workers[0].MinInstances = 10
		got, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.NoError(t, err)
		requireAsk(t, resources("cpu", "6", "memory", "24Gi", "nvidia.com/gpu", "1"), got.Ask)
	})

	t.Run("the cache cannot be read", func(t *testing.T) {
		tr := newTestTracker(t, newMapper(false), interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("cache stopped")
			},
		})
		tr.recordAssignment(newJob("job-1", 1), "cluster-1", _now)
		_, err := tr.inFlight(ctx, _kueueCluster, _now)
		require.ErrorContains(t, err, "get RayCluster team-a/job-1: cache stopped")
	})
}
