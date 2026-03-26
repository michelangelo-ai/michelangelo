package pipeline

import (
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.starlark.net/starlark"
)

const pluginID = "pipeline"

// Plugin is the global instance of the pipeline plugin for Starlark workflows.
var Plugin = &plugin{}

type plugin struct{}

var _ service.IPlugin = (*plugin)(nil)

func (r *plugin) ID() string {
	return pluginID
}

func (r *plugin) Create(runInfo service.RunInfo) starlark.Value {
	return newModule(runInfo)
}

func (r *plugin) Register(_ worker.Registry) {}
