package storage

import (
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(newPlugin),
)

const pluginID = "storage"

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

func newPlugin() *plugin {
	return &plugin{}
}
