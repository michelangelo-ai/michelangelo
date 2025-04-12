package spark

import (
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/worker"
)

const pluginID = "spark"

var Plugin = &plugin{}

type plugin struct{}

var _ cadstar.IPlugin = (*plugin)(nil)

func (r *plugin) ID() string {
	return pluginID
}
func (r *plugin) Create(_ cadstar.RunInfo) starlark.Value {
	return newModule()
}
func (r *plugin) Register(_ worker.Registry) {}
