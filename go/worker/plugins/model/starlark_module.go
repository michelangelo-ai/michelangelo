package model

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"

	model "github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
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
		"get_models_by_pipeline_run": starlark.NewBuiltin("get_models_by_pipeline_run", m.getModelsByPipelineRun).BindReceiver(m),
		"model_search":               starlark.NewBuiltin("model_search", m.modelSearch).BindReceiver(m),
	}
	return m
}

func (r *module) getModelsByPipelineRun(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace, pipelineRunName string
	if err := starlark.UnpackArgs("get_models_by_pipeline_run", args, kwargs,
		"namespace", &namespace,
		"pipeline_run_name", &pipelineRunName,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var response *model.GetModelsByPipelineRunResponse
	if err := workflow.ExecuteActivity(ctx, model.Activities.GetModelsByPipelineRun, &model.GetModelsByPipelineRunRequest{
		Namespace:       namespace,
		PipelineRunName: pipelineRunName,
	}).Get(ctx, &response); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var result starlark.Value
	if err := utils.AsStar(response.Models, &result); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return result, nil
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
