package cadstar

import (
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"
)

// RunInfo contextual info about the current run
type RunInfo struct {
	Info    *workflow.Info
	Environ *starlark.Dict
}

// IPlugin plugin factory interface
type IPlugin interface {
	// Create instantiates starlark.StringDict that exposes plugin's functions and properties
	Create(info RunInfo) starlark.StringDict
	// Register is deprecated and removed in Temporal.
}

// PluginFactory is a functional IPlugin implementation
type PluginFactory func(info RunInfo) starlark.StringDict

var _ IPlugin = (PluginFactory)(nil)

func (r PluginFactory) Create(info RunInfo) starlark.StringDict {
	return r(info)
}
