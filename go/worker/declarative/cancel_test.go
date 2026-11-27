package declarative

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	cadclient "go.uber.org/cadence"
	"go.uber.org/cadence/activity"
	"go.uber.org/cadence/encoded"
	"go.uber.org/cadence/testsuite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachedoutputact "github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	rayact "github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestCancelWhileClustersStart cancels a run while its ray tasks wait for
// their clusters to become ready. No exit hook is registered at that point,
// so only the cleanup of each create call can terminate its cluster. The
// workflow must stay open until each cleanup completes, and a task that the
// concurrency cap still holds back must not start.
func TestCancelWhileClustersStart(t *testing.T) {
	for _, tc := range []struct {
		name string
		cap  int
		want []string
	}{
		{name: "both_tasks_wait_for_a_cluster", cap: 2, want: []string{"rc-1", "rc-2"}},
		{name: "second_task_waits_for_the_cap", cap: 1, want: []string{"rc-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created, terminated := cancelWhileClustersStart(t, tc.cap)
			assert.Equal(t, len(tc.want), created, "a task started after the cancel")
			assert.Equal(t, tc.want, terminated, "the workflow closed before the cluster cleanup completed")
		})
	}
}

func cancelWhileClustersStart(t *testing.T, maxConcurrency int) (int, []string) {
	var terminated []string
	created := 0
	setup := func(env *testsuite.TestWorkflowEnvironment) {
		env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
		env.RegisterActivity(rayact.Activities.CreateRayCluster)
		env.RegisterActivity(rayact.Activities.SensorRayClusterReadiness)
		env.RegisterActivity(rayact.Activities.TerminateCluster)

		env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
			Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)

		env.OnActivity(rayact.Activities.CreateRayCluster, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.CreateRayClusterRequest) (*rayact.CreateRayClusterActivityResponse, error) {
				created++
				c := *req.RayCluster
				c.Name = fmt.Sprintf("rc-%d", created)
				return &rayact.CreateRayClusterActivityResponse{RayCluster: &c, ActivityID: c.Name}, nil
			})
		env.OnActivity(rayact.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).After(10 * time.Hour).Return(
			func(_ context.Context, req v2pb.GetRayClusterRequest) (*rayact.SensorRayClusterReadinessResponse, error) {
				c := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
				c.Status.State = v2pb.RAY_CLUSTER_STATE_READY
				return &rayact.SensorRayClusterReadinessResponse{RayCluster: c, Ready: true}, nil
			})

		// The delay keeps each terminate call open after the decision that
		// schedules it, so a workflow that closes in that decision never
		// sees it complete.
		env.OnActivity(rayact.Activities.TerminateCluster, mock.Anything, mock.Anything).After(time.Minute).Return(
			func(_ context.Context, req rayact.TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
				c := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}
				c.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
				return &v2pb.UpdateRayClusterResponse{RayCluster: c}, nil
			})
		env.SetOnActivityCompletedListener(func(info *activity.Info, result encoded.Value, err error) {
			if !strings.HasSuffix(info.ActivityType.Name, "TerminateCluster") || err != nil {
				return
			}
			var res v2pb.UpdateRayClusterResponse
			if result.Get(&res) == nil && res.RayCluster != nil {
				terminated = append(terminated, res.RayCluster.Name)
			}
		})
		env.RegisterDelayedCallback(env.CancelWorkflow, time.Minute)
	}

	rayTask := func(ep string) map[string]any {
		return map[string]any{"task_function": "core/ray", "depends_on": []any{}, "config": map[string]any{"entry_point": ep}, "job_specs": nil, "retry": nil}
	}
	environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
	env := execStar(t, setup, environ, casesTar(t), "/ray_cases.star", "run_ray",
		spec(map[string]any{"max_concurrency": maxConcurrency}, map[string]any{"first": rayTask("first.py"), "second": rayTask("second.py")}), map[string]any{})

	var ce *cadclient.CustomError
	require.ErrorAs(t, env.GetWorkflowError(), &ce)
	var details map[string]any
	require.NoError(t, ce.Details(&details))
	assert.Contains(t, details["error"], "CanceledError")

	sort.Strings(terminated)
	return created, terminated
}
