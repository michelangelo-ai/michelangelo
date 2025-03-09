package concurrent

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/cad"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"
)

var pluginID = "concurrent"
var Plugin = cadstar.PluginFactory(func(info cadstar.RunInfo) starlark.StringDict {
	return starlark.StringDict{pluginID: &plugin{}}
})

type plugin struct{}

var _ starlark.HasAttrs = &plugin{}

func (f *plugin) String() string                        { return pluginID }
func (f *plugin) Type() string                          { return pluginID }
func (f *plugin) Freeze()                               {}
func (f *plugin) Truth() starlark.Bool                  { return true }
func (f *plugin) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *plugin) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *plugin) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var builtins = map[string]*starlark.Builtin{
	"run": starlark.NewBuiltin("run", run),
}

var properties = map[string]star.PropertyFactory{}

func run(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ctx = cadstar.GetContext(t)

	future, settable := workflow.NewFuture(ctx)

	fn := args[0]
	args = args[1:]

	workflow.Go(ctx, func(ctx workflow.Context) {
		t := cadstar.CreateThread(ctx)
		settable.Set(starlark.Call(t, fn, args, kwargs))
	})

	return &cad.Future{Future: future}, nil
}
