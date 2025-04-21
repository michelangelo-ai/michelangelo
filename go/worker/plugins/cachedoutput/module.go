package cachedoutput

import (
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/worker"
)

const pluginID = "cachedoutput"

// Plugin is the plugin for the cachedoutput module.
var Plugin = &plugin{}

type plugin struct{}

var _ cadstar.IPlugin = (*plugin)(nil)

// ID returns the ID of the cachedoutput plugin.
func (r *plugin) ID() string {
	return pluginID
}

// Create creates a new instance of the cachedoutput module.
func (r *plugin) Create(info cadstar.RunInfo) starlark.Value {
	return &Module{info: info.Info}
}

// Register registers the cachedoutput module with the worker registry.
func (r *plugin) Register(registry worker.Registry) {}
