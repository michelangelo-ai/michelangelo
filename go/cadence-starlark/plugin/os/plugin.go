package os

import (
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"go.starlark.net/starlark"
)

var Plugin = cadstar.PluginFactory(func(info cadstar.RunInfo) starlark.StringDict {
	return starlark.StringDict{"os": &Module{environ: info.Environ}}
})
