package storage

import (
	"fmt"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/ext"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"

	"github.com/michelangelo-ai/michelangelo/go/temporalworker/activities/storage"
)

// Module FX
var Module = fx.Provide(create)

// Plugin is the default cadstar.IPlugin implementation for the Ray plugin
var Plugin = cadstar.PluginFactory(func(_ cadstar.RunInfo) starlark.StringDict {
	return starlark.StringDict{pluginID: &plugin{}}
})

type out struct {
	fx.Out
	Plugin cadstar.IPlugin `group:"plugin"`
}

func create() out {
	return out{Plugin: Plugin}
}

const pluginID = "storage"

type plugin struct{}

var _ starlark.HasAttrs = &plugin{}

func (f *plugin) String() string                        { return pluginID }
func (f *plugin) Type() string                          { return pluginID }
func (f *plugin) Freeze()                               {}
func (f *plugin) Truth() starlark.Bool                  { return true }
func (f *plugin) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *plugin) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *plugin) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var properties = map[string]star.PropertyFactory{}

var builtins = map[string]*starlark.Builtin{
	"read": starlark.NewBuiltin("create_cluster", read),
}

func read(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var protocol string
	var path string
	if err := starlark.UnpackArgs("execute", args, kwargs,
		"protocol", &protocol,
		"path", &path,
	); err != nil {
		logger.Error("builtin-error", ext.ErrorFields(err)...)
		return nil, err
	}

	var res any
	if err := workflow.ExecuteActivity(ctx, storage.Activities.Read, protocol, path).Get(ctx, &res); err != nil {
		logger.Error("builtin-error", ext.ErrorFields(err)...)
		return nil, err
	}
	var ret starlark.Value
	if err := star.AsStar(res, &ret); err != nil {
		logger.Error("builtin-error", ext.ErrorFields(err)...)
		return nil, err
	}
	return ret, nil
}
