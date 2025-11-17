package ray

import (
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.starlark.net/starlark"
)

const pluginID = "ray"

// Plugin is the global instance of the Ray plugin for Starlark workflows.
var Plugin = &plugin{}

type plugin struct{}

var _ service.IPlugin = (*plugin)(nil)

func (r *plugin) ID() string {
	return pluginID
}
func (r *plugin) Create(_ service.RunInfo) starlark.Value {
	return newModule()
}
func (r *plugin) Register(_ worker.Registry) {}
