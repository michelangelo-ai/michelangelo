package atexit

import (
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
)

var pluginID = "atexit"
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
	"register":   starlark.NewBuiltin("register", register),
	"unregister": starlark.NewBuiltin("unregister", unregister),
}

var properties = map[string]star.PropertyFactory{}

func register(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	fn := args[0].(starlark.Callable)
	args = args[1:]
	cadstar.GetExitHooks(ctx).Register(fn, args, kwargs)
	return starlark.None, nil
}

func unregister(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	fn := args[0].(starlark.Callable)
	cadstar.GetExitHooks(ctx).Unregister(fn)
	return starlark.None, nil
}
