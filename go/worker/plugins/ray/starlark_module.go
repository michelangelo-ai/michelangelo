package ray

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
)

// TODO: andrii: implement Ray starlark plugin here

var _ starlark.HasAttrs = (*module)(nil)

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_cluster": starlark.NewBuiltin("create_cluster", m.createCluster).BindReceiver(m),
		"create_job":     starlark.NewBuiltin("create_job", m.createJob).BindReceiver(m),
	}
	return m
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

func (r *module) createCluster(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var spec *starlark.Dict
	if err := starlark.UnpackArgs("create_cluster", args, kwargs, "spec", &spec); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	var response starlark.Value
	if err := workflow.ExecuteActivity(ctx, ray.Activities.CreateRayCluster, spec).Get(ctx, &response); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	return response, nil
}

func (r *module) createJob(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var spec *starlark.Dict
	if err := starlark.UnpackArgs("create_job", args, kwargs, "spec", &spec); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	return spec, nil
}
