package declarative

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/cadence/testsuite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pipelinedecl "github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	cachedoutputact "github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	rayact "github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	sparkact "github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// releaseArgs checks spec against the golden release as the apiserver does,
// and encodes it as the controller does.
func releaseArgs(t *testing.T, spec string) []any {
	t.Helper()
	w := &v2pb.DeclarativeWorkflow{}
	require.NoError(t, jsonpb.Unmarshal(strings.NewReader(spec), w))
	require.NoError(t, pipelinedecl.ValidateAgainst(w, pipelinedecl.DefaultLimits(), parseRelease(t)))
	args, err := pipelinedecl.WorkflowArgs(w)
	require.NoError(t, err)
	return args
}

// jobSpecsTask returns a task of taskType that sets every job_specs field
// that the golden release lists under key. A field with no value in known
// gets a value of its type. Thus a schema field that the factory does not
// accept still reaches the factory, and the task does not start.
func jobSpecsTask(t *testing.T, taskType, key string, known map[string]any) string {
	t.Helper()
	fields := parseRelease(t).TaskTypes[taskType].JobSpecs[key].Fields
	require.NotEmpty(t, fields)
	byType := map[string]any{"string": "1G", "integer": 1, "number": 1.5, "boolean": true, "list": []any{}, "dict": map[string]any{}}
	values := map[string]any{}
	for name, field := range fields {
		if v, ok := known[name]; ok {
			values[name] = v
			continue
		}
		v, ok := byType[field.Type]
		require.True(t, ok, "job_specs.%s.%s has type %q", key, name, field.Type)
		values[name] = v
	}
	for name := range known {
		require.Contains(t, fields, name, "the release has no job_specs.%s.%s", key, name)
	}
	spec, err := json.Marshal(map[string]any{
		"schema_version": "v1alpha1",
		"tasks": map[string]any{"t": map[string]any{
			"task_function": taskType,
			"config":        map[string]any{"entry_point": "t.py"},
			"job_specs":     map[string]any{key: values},
		}},
	})
	require.NoError(t, err)
	return string(spec)
}

func resources(c corev1.Container) map[string]string {
	out := map[string]string{}
	for name, q := range c.Resources.Requests {
		out["requests."+string(name)] = q.String()
	}
	for name, q := range c.Resources.Limits {
		out["limits."+string(name)] = q.String()
	}
	return out
}

// TestRayResourcesReachCluster sets every core/ray job_specs field that the
// golden release lists, and runs the golden release. The object store values are above 2^31 to catch a
// float or int32 on the way.
func TestRayResourcesReachCluster(t *testing.T) {
	args := releaseArgs(t, jobSpecsTask(t, "core/ray", "ray", map[string]any{
		"head_cpu": 3, "head_memory": "5Gi", "head_disk": "7Gi", "head_gpu": 1,
		"head_object_store_memory": 3000000001,
		"worker_cpu":               2, "worker_memory": "6Gi", "worker_disk": "9Gi", "worker_gpu": 2,
		"worker_object_store_memory": 2500000003, "worker_instances": 4,
	}))

	var cluster *v2pb.RayCluster
	setup := func(env *testsuite.TestWorkflowEnvironment) {
		env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
		env.RegisterActivity(rayact.Activities.CreateRayCluster)
		env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
			Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)
		env.OnActivity(rayact.Activities.CreateRayCluster, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.CreateRayClusterRequest) (*rayact.CreateRayClusterActivityResponse, error) {
				cluster = req.RayCluster
				return &rayact.CreateRayClusterActivityResponse{ActivityID: "create"}, nil
			})
	}
	environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
	execStar(t, setup, environ, readFile(t, releaseTar), "", "", args...)

	require.NotNil(t, cluster, "the ray factory did not create a cluster; does it accept every job_specs field?")
	head := cluster.Spec.Head
	assert.Equal(t, map[string]string{
		"requests.cpu": "3", "requests.memory": "5Gi", "requests.ephemeral-storage": "7Gi",
		"requests.nvidia.com/gpu": "1", "limits.nvidia.com/gpu": "1",
	}, resources(head.Pod.Spec.Containers[0]))
	assert.Equal(t, "3000000001", head.RayStartParams["object-store-memory"])

	require.Len(t, cluster.Spec.Workers, 1)
	worker := cluster.Spec.Workers[0]
	assert.Equal(t, map[string]string{
		"requests.cpu": "2", "requests.memory": "6Gi", "requests.ephemeral-storage": "9Gi",
		"requests.nvidia.com/gpu": "2", "limits.nvidia.com/gpu": "2",
	}, resources(worker.Pod.Spec.Containers[0]))
	assert.Equal(t, "2500000003", worker.RayStartParams["object-store-memory"])
	assert.Equal(t, int32(4), worker.MinInstances)
	assert.Equal(t, int32(4), worker.MaxInstances)
}

// TestSparkResourcesReachJob sets every core/spark job_specs field that the
// golden release lists, and runs the golden release.
func TestSparkResourcesReachJob(t *testing.T) {
	args := releaseArgs(t, jobSpecsTask(t, "core/spark", "spark", map[string]any{
		"driver_cpu": 3, "driver_memory": "5G", "driver_disk": "7G", "driver_gpu": 1,
		"executor_cpu": 2, "executor_memory": "6G", "executor_disk": "9G", "executor_gpu": 2,
		"executor_instances": 4,
	}))

	var job *v2pb.SparkJob
	setup := func(env *testsuite.TestWorkflowEnvironment) {
		env.RegisterActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry)
		env.RegisterActivity(sparkact.Activities.CreateSparkJob)
		env.RegisterActivity(sparkact.Activities.SensorSparkJob)
		env.OnActivity(cachedoutputact.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).
			Return(&cachedoutputact.ShouldOverrideCacheForRetryResponse{}, nil)
		env.OnActivity(sparkact.Activities.CreateSparkJob, mock.Anything, mock.Anything).Return(
			func(_ context.Context, req v2pb.CreateSparkJobRequest) (*sparkact.CreateSparkJobActivityResponse, error) {
				job = req.SparkJob
				j := *req.SparkJob
				j.Name = "sp-t"
				return &sparkact.CreateSparkJobActivityResponse{SparkJob: &j, ActivityID: "create"}, nil
			})
		failed := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "sp-t"}}
		failed.Status.StatusConditions = []*apipb.Condition{{Type: "Succeeded", Status: apipb.CONDITION_STATUS_FALSE}}
		env.OnActivity(sparkact.Activities.SensorSparkJob, mock.Anything, mock.Anything).
			Return(&sparkact.SensorSparkJobResponse{SparkJob: failed, Terminal: true}, nil)
	}
	environ := map[string]any{"UF_TASK_IMAGE": "img:1", "UF_STORAGE_URL": "s3://bucket/run", "MA_NAMESPACE": "ns"}
	execStar(t, setup, environ, readFile(t, releaseTar), "", "", args...)

	require.NotNil(t, job, "the spark factory did not create a job; does it accept every job_specs field?")
	assert.Equal(t, &v2pb.ResourceSpec{Cpu: 3, Memory: "5G", DiskSize: "7G", Gpu: 1}, job.Spec.Driver.Pod.Resource)
	assert.Equal(t, &v2pb.ResourceSpec{Cpu: 2, Memory: "6G", DiskSize: "9G", Gpu: 2}, job.Spec.Executor.Pod.Resource)
	assert.Equal(t, int32(4), job.Spec.Executor.Instances)
}
