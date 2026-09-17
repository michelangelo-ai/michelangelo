package model

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"

	model "github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var (
	_    starlark.HasAttrs = (*module)(nil)
	poll int64             = 10
)

type module struct {
	attributes map[string]starlark.Value
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"deploy_model": starlark.NewBuiltin("deploy_model", m.deployModel).BindReceiver(m),
		"model_search": starlark.NewBuiltin("model_search", m.modelSearch).BindReceiver(m),
	}
	return m
}

// deployModel resolves a model produced by a child pipeline run, creates or updates
// its Deployment, and waits for that exact model to become healthy.
func (r *module) deployModel(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace string
	var deploymentName string
	var pipelineRunName string
	var inferenceServerName string
	var modelName string
	var actor string
	var timeoutSeconds int64
	var pollSeconds int64 = poll
	if err := starlark.UnpackArgs("deploy_model", args, kwargs,
		"namespace", &namespace,
		"deployment_name", &deploymentName,
		"pipeline_run_name", &pipelineRunName,
		"inference_server_name", &inferenceServerName,
		"model_name?", &modelName,
		"actor?", &actor,
		"timeout_seconds?", &timeoutSeconds,
		"poll_seconds?", &pollSeconds,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	if namespace == "" || deploymentName == "" || pipelineRunName == "" || inferenceServerName == "" {
		return nil, fmt.Errorf("namespace, deployment_name, pipeline_run_name, and inference_server_name must be non-empty")
	}
	if timeoutSeconds < 0 {
		return nil, fmt.Errorf("timeout_seconds must be non-negative")
	}
	if pollSeconds <= 0 {
		return nil, fmt.Errorf("poll_seconds must be positive")
	}

	request := &model.DeployModelRequest{
		Namespace:           namespace,
		DeploymentName:      deploymentName,
		PipelineRunName:     pipelineRunName,
		InferenceServerName: inferenceServerName,
		ModelName:           modelName,
		Actor:               actor,
	}
	var deployResponse *model.DeployModelResponse
	if err := workflow.ExecuteActivity(ctx, model.Activities.DeployModel, request).Get(ctx, &deployResponse); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	if deployResponse == nil || deployResponse.Deployment == nil || deployResponse.ModelName == "" {
		return nil, fmt.Errorf("deploy model activity returned an incomplete response")
	}

	if timeoutSeconds == 0 {
		timeoutSeconds = int64(utils.LongTimeout.Seconds())
	}
	retryPolicy := utils.DefaultSensorRetryPolicy
	retryPolicy.ExpirationInterval = time.Duration(timeoutSeconds) * time.Second
	retryPolicy.InitialInterval = time.Duration(pollSeconds) * time.Second
	sensorCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	var deployment *v2pb.Deployment
	if err := workflow.ExecuteActivity(sensorCtx, model.Activities.DeploymentSensor, &model.DeploymentSensorRequest{
		Namespace:      namespace,
		DeploymentName: deploymentName,
		ModelName:      deployResponse.ModelName,
	}).Get(sensorCtx, &deployment); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	switch deployment.Status.Stage {
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		detail := ""
		if deployment.Status.Message != "" {
			detail = ": " + deployment.Status.Message
		}
		return nil, fmt.Errorf("deployment %s/%s ended in %s%s", namespace, deploymentName, deployment.Status.Stage.String(), detail)
	}

	result := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      deployment.GetName(),
			"namespace": deployment.GetNamespace(),
		},
		"model": map[string]interface{}{
			"name":      deployResponse.ModelName,
			"namespace": namespace,
		},
		"status": map[string]interface{}{
			"state": deployment.Status.State.String(),
			"stage": deployment.Status.Stage.String(),
		},
	}
	var resultValue starlark.Value
	if err := utils.AsStar(result, &resultValue); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return resultValue, nil
}

func (r *module) modelSearch(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace string
	var deploymentName string
	if err := starlark.UnpackArgs("model_search", args, kwargs,
		"namespace", &namespace,
		"deployment_name", &deploymentName,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Execute ModelSearch activity
	var response *starlark.Dict
	if err := workflow.ExecuteActivity(ctx, model.Activities.ModelSearch, &model.ModelSearchRequest{
		Namespace:      namespace,
		DeploymentName: deploymentName,
	}).Get(ctx, &response); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	return response, nil
}
