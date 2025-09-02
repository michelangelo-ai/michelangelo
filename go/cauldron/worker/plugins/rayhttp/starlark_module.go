package rayhttp

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/activities/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/compute/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"go.starlark.net/starlark"
	"go.uber.org/zap"
)

// Module struct implements starlark.HasAttrs interface
var _ starlark.HasAttrs = (*module)(nil)

var poll int64 = 10

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_job": starlark.NewBuiltin("create_job", m.createRayJob).BindReceiver(m),
	}
	return m
}

func (r *module) String() string        { return pluginID }
func (r *module) Type() string          { return pluginID }
func (r *module) Freeze()               {}
func (r *module) Truth() starlark.Bool  { return true }
func (r *module) Hash() (uint32, error) { return starlark.String(pluginID).Hash() }
func (r *module) Attr(name string) (starlark.Value, error) {
	if val, ok := r.attributes[name]; ok {
		return val, nil
	}
	return nil, nil
}
func (r *module) AttrNames() []string {
	return ext.SortedKeys(r.attributes)
}

// createRayJob creates a new Ray job via the HTTP API and waits for it to be ready.
func (r *module) createRayJob(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(thread)
	logger := workflow.GetLogger(ctx)

	var rayJobSpec *starlark.Dict
	var userToken string
	if err := starlark.UnpackArgs("create_job", args, kwargs, "ray_job_spec", &rayJobSpec, "user_token", &userToken); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}

	// Convert the provided ray job spec from starlark to worker/ray object
	var rayJob ray.RayJob
	if err := utils.AsGo(rayJobSpec, &rayJob); err != nil {
		logger.Error("error converting ray job spec to worker/ray object", zap.Error(err))
		return nil, err
	}

	// Extract pipeline from HeadGroupSpec environment variables
	var pipeline string
	if rayJob.Spec.RayClusterSpec.HeadGroupSpec.Env != nil {
		for _, env := range rayJob.Spec.RayClusterSpec.HeadGroupSpec.Env {
			if env.Name == "MLP_PIPELINE" {
				pipeline = env.Value
				break
			}
		}
	}

	// First build the Ray job image
	buildImageRequest := rayhttp.BuildRayJobImageRequest{
		Object: map[string]interface{}{
			"apiVersion": "ml.chimera.kubebuilder.io/v1",
			"kind":       "UniflowTask",
			"metadata": map[string]interface{}{
				"name": strings.Split(rayJob.Metadata.Name, "-")[len(strings.Split(rayJob.Metadata.Name, "-"))-1],
			},
			"spec": map[string]interface{}{
				"pipeline": pipeline,
			},
		},
		UsePipelineImage: false,
		CommitHash:       "test", // You may want to make this configurable
		UserToken:        userToken,
	}

	var buildResponse rayhttp.BuildRayJobImageResponse
	err := workflow.ExecuteActivity(ctx, rayhttp.Activities.BuildRayJobImage, buildImageRequest).Get(ctx, &buildResponse)
	if err != nil {
		logger.Error("error executing build image activity", zap.Error(err))
		return nil, err
	}

	// Sense image build completion
	sensorRequest := rayhttp.SensorRayJobImageRequest{
		JobName:   buildResponse.JobName,
		UserToken: userToken,
	}

	var sensorResponse rayhttp.SensorRayJobImageResponse

	// Sense until image build is complete
	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	err = workflow.ExecuteActivity(sensorCtx, rayhttp.Activities.SensorRayJobImage, sensorRequest).Get(sensorCtx, &sensorResponse)
	if err != nil {
		logger.Error("error executing poll image build activity", zap.Error(err))
		return nil, err
	}

	// Update the ray job with the built image
	rayJob.Spec.RayClusterSpec.Image = buildResponse.ImageRegistry

	// Marshal the ray job into the request format expected by activities
	rayJobBytes, err := json.Marshal(rayJob)
	if err != nil {
		logger.Error("error marshaling ray job", zap.Error(err))
		return nil, err
	}

	var request struct {
		RayJob    json.RawMessage `json:"rayJob"`
		UserToken string          `json:"userToken"`
	}
	request.RayJob = rayJobBytes
	request.UserToken = userToken

	// Execute the create activity
	var createResponse rayhttp.CreateRayJobResponse
	srp = utils.CadenceDefaultRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	createCtx := workflow.WithRetryPolicy(ctx, srp)
	err = workflow.ExecuteActivity(createCtx, rayhttp.Activities.CreateRayJob, request).Get(ctx, &createResponse)
	if err != nil {
		logger.Error("error executing create activity", zap.Error(err))
		return nil, err
	}

	name := createResponse.Object["metadata"].(map[string]interface{})["name"].(string)

	// Now poll for the job to be ready
	jobSensorRequest := struct {
		Name      string `json:"name"`
		UserToken string `json:"userToken"`
	}{
		Name:      name,
		UserToken: userToken,
	}

	// Set up polling with retry policy
	srp = utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	jobSensorCtx := workflow.WithRetryPolicy(ctx, srp)

	// Monitor job until it's in a terminal state
	var getResponse rayhttp.GetRayJobResponse

	if err := workflow.ExecuteActivity(jobSensorCtx, rayhttp.Activities.SensorRayJob, jobSensorRequest).Get(jobSensorCtx, &getResponse); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var result starlark.Value
	if err := utils.AsStar(getResponse.Object, &result); err != nil {
		logger.Error("error converting to Starlark", zap.Error(err))
		return nil, err
	}

	return result, nil
}
