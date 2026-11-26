package declarative

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/cadence/testsuite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachedoutputact "github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	rayact "github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestRayExitHooksPerTask runs two ray tasks in one level. The job of the
// second cluster fails first, while the job of the first cluster still runs.
// A cancel must then terminate the first cluster through its exit hook.
func TestRayExitHooksPerTask(t *testing.T) {
	var terminated []string
	setup := func(env *testsuite.TestWorkflowEnvironment) {
		env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
		env.RegisterActivity(rayact.Activities.CreateRayCluster)
		env.RegisterActivity(rayact.Activities.SensorRayClusterReadiness)
		env.RegisterActivity(rayact.Activities.CreateRayJob)
		env.RegisterActivity(rayact.Activities.SensorRayJob)
		env.RegisterActivity(rayact.Activities.TerminateCluster)

		env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
			Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)

		created := 0
		env.OnActivity(rayact.Activities.CreateRayCluster, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.CreateRayClusterRequest) (*rayact.CreateRayClusterActivityResponse, error) {
				created++
				c := *req.RayCluster
				c.Name = []string{"", "rc-first", "rc-second"}[created]
				return &rayact.CreateRayClusterActivityResponse{RayCluster: &c, ActivityID: c.Name}, nil
			})
		env.OnActivity(rayact.Activities.SensorRayClusterReadiness, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.GetRayClusterRequest) (*rayact.SensorRayClusterReadinessResponse, error) {
				c := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
				c.Status.State = v2pb.RAY_CLUSTER_STATE_READY
				return &rayact.SensorRayClusterReadinessResponse{RayCluster: c, Ready: true}, nil
			})
		env.OnActivity(rayact.Activities.CreateRayJob, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.CreateRayJobRequest) (*v2pb.CreateRayJobResponse, error) {
				j := *req.RayJob
				j.Name = j.Spec.Cluster.Name + "-job"
				return &v2pb.CreateRayJobResponse{RayJob: &j}, nil
			})
		job := func(name string, state v2pb.RayJobState) *rayact.SensorRayJobResponse {
			j := &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: name}}
			j.Status.State = state
			return &rayact.SensorRayJobResponse{RayJob: j, Terminal: true}
		}
		env.OnActivity(rayact.Activities.SensorRayJob, mock.Anything, mock.MatchedBy(func(r v2pb.GetRayJobRequest) bool { return r.Name == "rc-first-job" })).
			After(10*time.Hour).Return(job("rc-first-job", v2pb.RAY_JOB_STATE_SUCCEEDED), nil)
		env.OnActivity(rayact.Activities.SensorRayJob, mock.Anything, mock.MatchedBy(func(r v2pb.GetRayJobRequest) bool { return r.Name == "rc-second-job" })).
			Return(job("rc-second-job", v2pb.RAY_JOB_STATE_FAILED), nil)
		env.OnActivity(rayact.Activities.TerminateCluster, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req rayact.TerminateClusterRequest) (*v2pb.UpdateRayClusterResponse, error) {
				terminated = append(terminated, req.Name)
				c := &v2pb.RayCluster{}
				c.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
				return &v2pb.UpdateRayClusterResponse{RayCluster: c}, nil
			})
		env.RegisterDelayedCallback(env.CancelWorkflow, time.Hour)
	}

	rayTask := func(ep string) map[string]any {
		return map[string]any{"task_function": "core/ray", "depends_on": []any{}, "config": map[string]any{"entry_point": ep}, "job_specs": nil, "retry": nil}
	}
	environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
	execStar(t, setup, environ, casesTar(t), "/ray_cases.star", "run_ray",
		spec(nil, map[string]any{"first": rayTask("first.py"), "second": rayTask("second.py")}), map[string]any{})

	assert.Equal(t, []string{"rc-second", "rc-first"}, terminated)
}
