package ray

import (
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"go.starlark.net/starlark"
	"go.uber.org/fx"
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
