package declarative

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	cadclient "go.uber.org/cadence"
	"go.uber.org/cadence/testsuite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachedoutputact "github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	rayact "github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	sparkact "github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	storageact "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
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

// TestSparkExitHooksPerTask runs two spark tasks in one level. The second job
// succeeds and unregisters its exit hook while the first job still runs. A
// cancel must then run the exit hook of the first job, not of the second.
func TestSparkExitHooksPerTask(t *testing.T) {
	var terminated, sensedAfterCancel []string
	canceled := false
	setup := func(env *testsuite.TestWorkflowEnvironment) {
		env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
		env.RegisterActivity(cachedoutputact.Activities.CreateCachedOutput)
		env.RegisterActivity(storageact.Activities.Read)
		env.RegisterActivity(sparkact.Activities.CreateSparkJob)
		env.RegisterActivity(sparkact.Activities.SensorSparkJob)
		env.RegisterActivity(sparkact.Activities.TerminateSparkJob)

		env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
			Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)
		env.OnActivity(cachedoutputact.Activities.CreateCachedOutput, mock.Anything, mock.Anything).
			Return(&v2pb.CreateCachedOutputResponse{CachedOutput: &v2pb.CachedOutput{}}, nil)
		env.OnActivity(storageact.Activities.Read, mock.Anything, mock.Anything).Return(map[string]any{}, nil)

		created := func(_ context.Context, req v2pb.CreateSparkJobRequest) (*sparkact.CreateSparkJobActivityResponse, error) {
			j := *req.SparkJob
			j.Name = "sp-" + strings.TrimSuffix(j.Spec.MainArgs[1], ".py")
			return &sparkact.CreateSparkJobActivityResponse{SparkJob: &j, ActivityID: j.Name}, nil
		}
		task := func(name string) any {
			return mock.MatchedBy(func(r v2pb.CreateSparkJobRequest) bool { return r.SparkJob.Spec.MainArgs[1] == name })
		}
		// The first job registers its hook first, so the old code removed it
		// when the second job unregistered.
		env.OnActivity(sparkact.Activities.CreateSparkJob, mock.Anything, task("first.py")).Return(created)
		env.OnActivity(sparkact.Activities.CreateSparkJob, mock.Anything, task("second.py")).After(time.Minute).Return(created)
		succeeded := func(name string) *sparkact.SensorSparkJobResponse {
			j := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: name}}
			j.Status.StatusConditions = []*apipb.Condition{{Type: "Succeeded", Status: apipb.CONDITION_STATUS_TRUE}}
			return &sparkact.SensorSparkJobResponse{SparkJob: j, Terminal: true}
		}
		sensed := func(args mock.Arguments) {
			if canceled {
				sensedAfterCancel = append(sensedAfterCancel, args.Get(1).(v2pb.GetSparkJobRequest).Name)
			}
		}
		env.OnActivity(sparkact.Activities.SensorSparkJob, mock.Anything, mock.MatchedBy(func(r v2pb.GetSparkJobRequest) bool { return r.Name == "sp-first" })).
			Run(sensed).After(10*time.Hour).Return(succeeded("sp-first"), nil)
		env.OnActivity(sparkact.Activities.SensorSparkJob, mock.Anything, mock.MatchedBy(func(r v2pb.GetSparkJobRequest) bool { return r.Name == "sp-second" })).
			Run(sensed).Return(succeeded("sp-second"), nil)
		env.OnActivity(sparkact.Activities.TerminateSparkJob, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req sparkact.TerminateSparkJobRequest) (*v2pb.UpdateSparkJobResponse, error) {
				terminated = append(terminated, req.Name)
				return &v2pb.UpdateSparkJobResponse{SparkJob: &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}}, nil
			})
		env.RegisterDelayedCallback(func() {
			canceled = true
			env.CancelWorkflow()
		}, time.Hour)
	}

	sparkTask := func(ep string) map[string]any {
		return map[string]any{"task_function": "core/spark", "depends_on": []any{}, "config": map[string]any{"entry_point": ep}, "job_specs": nil, "retry": nil}
	}
	environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
	execStar(t, setup, environ, casesTar(t), "/spark_cases.star", "run_spark",
		spec(nil, map[string]any{"first": sparkTask("first.py"), "second": sparkTask("second.py")}), map[string]any{})

	assert.Equal(t, []string{"sp-first"}, terminated)
	assert.Contains(t, sensedAfterCancel, "sp-first")
	assert.NotContains(t, sensedAfterCancel, "sp-second", "the exit hook of the finished job ran instead")
}

// TestSparkExitHooksPerAttempt runs one spark task with two attempts. A
// failed attempt that a retry follows must leave no exit hook: at the end of
// the workflow, that hook reports the failed job again and fails the run.
func TestSparkExitHooksPerAttempt(t *testing.T) {
	for _, tc := range []struct {
		name string
		// second is the Succeeded condition status of the second attempt.
		second  apipb.ConditionStatus
		wantErr string
		states  []string
	}{
		{
			name:   "retry then success",
			second: apipb.CONDITION_STATUS_TRUE,
			states: []string{"PENDING", "RUNNING", "FAILED", "PENDING", "RUNNING", "SUCCEEDED", "SUCCEEDED"},
		},
		{
			// The exit hook of the last attempt is the only source of the
			// failure reason in the workflow error.
			name:    "all attempts fail",
			second:  apipb.CONDITION_STATUS_FALSE,
			wantErr: "spark job failed: OOM driver ran out of memory",
			states:  []string{"PENDING", "RUNNING", "FAILED", "PENDING", "RUNNING", "FAILED", "FAILED"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sensed := map[string]int{}
			setup := func(env *testsuite.TestWorkflowEnvironment) {
				env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
				env.RegisterActivity(cachedoutputact.Activities.CreateCachedOutput)
				env.RegisterActivity(storageact.Activities.Read)
				env.RegisterActivity(sparkact.Activities.CreateSparkJob)
				env.RegisterActivity(sparkact.Activities.SensorSparkJob)

				env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
					Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)
				env.OnActivity(cachedoutputact.Activities.CreateCachedOutput, mock.Anything, mock.Anything).
					Return(&v2pb.CreateCachedOutputResponse{CachedOutput: &v2pb.CachedOutput{}}, nil)
				env.OnActivity(storageact.Activities.Read, mock.Anything, mock.Anything).Return(map[string]any{}, nil)

				attempt := 0
				env.OnActivity(sparkact.Activities.CreateSparkJob, mock.Anything, mock.Anything).Return(
					func(_ context.Context, req v2pb.CreateSparkJobRequest) (*sparkact.CreateSparkJobActivityResponse, error) {
						attempt++
						j := *req.SparkJob
						j.Name = []string{"", "sp-attempt-1", "sp-attempt-2"}[attempt]
						return &sparkact.CreateSparkJobActivityResponse{SparkJob: &j, ActivityID: j.Name}, nil
					})
				status := map[string]apipb.ConditionStatus{"sp-attempt-1": apipb.CONDITION_STATUS_FALSE, "sp-attempt-2": tc.second}
				env.OnActivity(sparkact.Activities.SensorSparkJob, mock.Anything, mock.Anything).Return(
					func(_ context.Context, req v2pb.GetSparkJobRequest) (*sparkact.SensorSparkJobResponse, error) {
						// Each attempt senses its job two times before the
						// workflow ends: until running, and until terminated.
						sensed[req.Name]++
						j := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}
						j.Status.StatusConditions = []*apipb.Condition{{Type: "Succeeded", Status: status[req.Name], Reason: "OOM", Message: "driver ran out of memory"}}
						return &sparkact.SensorSparkJobResponse{SparkJob: j, Terminal: true}, nil
					})
			}

			task := map[string]any{"task_function": "core/spark", "depends_on": []any{}, "config": map[string]any{"entry_point": "t.py"}, "job_specs": nil, "retry": map[string]any{"max_attempts": float64(2)}}
			environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
			env := execStar(t, setup, environ, casesTar(t), "/spark_cases.star", "run_spark", spec(nil, map[string]any{"t": task}), map[string]any{})

			q, err := env.QueryWorkflow("task_progress")
			require.NoError(t, err)
			var progress []string
			require.NoError(t, q.Get(&progress))
			var states []string
			for _, line := range progress {
				var p struct {
					TaskState string `json:"task_state"`
				}
				require.NoError(t, json.Unmarshal([]byte(line), &p), line)
				states = append(states, p.TaskState)
			}
			assert.Equal(t, tc.states, states)
			assert.Equal(t, 2, sensed["sp-attempt-1"], "the exit hook of the first attempt ran")

			if tc.wantErr == "" {
				assert.NoError(t, env.GetWorkflowError())
				return
			}
			var ce *cadclient.CustomError
			require.ErrorAs(t, env.GetWorkflowError(), &ce)
			var details map[string]any
			require.NoError(t, ce.Details(&details))
			assert.Contains(t, details["error"], tc.wantErr)
		})
	}
}
