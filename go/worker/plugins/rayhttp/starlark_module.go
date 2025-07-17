package rayhttp

import (
	"encoding/json"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"github.com/michelangelo-ai/michelangelo/go/worker/ray"
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
	if err := starlark.UnpackArgs("create_job", args, kwargs, "ray_job_spec", &rayJobSpec); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}

	// Convert the provided ray job spec from starlark to worker/ray object
	var rayJob ray.RayJob
	if err := utils.AsGo(rayJobSpec, &rayJob); err != nil {
		logger.Error("error converting ray job spec to worker/ray object", zap.Error(err))
		return nil, err
	}

	// Marshal the ray job into the request format expected by activities
	rayJobBytes, err := json.Marshal(rayJob)
	if err != nil {
		logger.Error("error marshaling ray job", zap.Error(err))
		return nil, err
	}

	var request struct {
		RayJob json.RawMessage `json:"rayJob"`
	}
	request.RayJob = rayJobBytes

	// Execute the create activity
	var createResponse ray.CreateRayJobResponse
	srp := utils.CadenceDefaultRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	createCtx := workflow.WithRetryPolicy(ctx, srp)
	err = workflow.ExecuteActivity(createCtx, rayhttp.Activities.CreateRayJob, request).Get(ctx, &createResponse)
	if err != nil {
		logger.Error("error executing create activity", zap.Error(err))
		return nil, err
	}

	name := createResponse.Object["metadata"].(map[string]interface{})["name"].(string)

	// Now poll for the job to be ready
	sensorRequest := struct {
		Name string `json:"name"`
	}{
		Name: name,
	}

	// Set up polling with retry policy
	srp = utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	// Monitor job until it's in a terminal state
	var getResponse ray.GetRayJobResponse

	if err := workflow.ExecuteActivity(sensorCtx, rayhttp.Activities.SensorRayJob, sensorRequest).Get(sensorCtx, &getResponse); err != nil {
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
