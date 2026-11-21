// Package buildkite exposes Buildkite orchestration to Starlark workflows.
package buildkite

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.starlark.net/starlark"
)

const pluginID = "buildkite"

var (
	// Plugin is the remote module bound by Python's @star_plugin functions.
	Plugin = &plugin{}

	builtins = map[string]*starlark.Builtin{
		"trigger_build":  starlark.NewBuiltin("trigger_build", triggerBuild),
		"get_build":      starlark.NewBuiltin("get_build", getBuild),
		"wait_for_build": starlark.NewBuiltin("wait_for_build", waitForBuild),
		"invoke_build":   starlark.NewBuiltin("invoke_build", invokeBuild),
		"cancel_build":   starlark.NewBuiltin("cancel_build", cancelBuild),
	}

	_ service.IPlugin   = (*plugin)(nil)
	_ starlark.HasAttrs = (*Module)(nil)
)

type plugin struct{}

// Module contains the workflow-safe Buildkite operations.
type Module struct{}

// ID returns the stable module ID.
func (*plugin) ID() string { return pluginID }

// Create constructs the module for an execution.
func (*plugin) Create(service.RunInfo) starlark.Value { return &Module{} }

// Register is empty because activities/buildkite.Module registers activities.
func (*plugin) Register(worker.Registry) {}

// String returns the module name.
func (*Module) String() string { return pluginID }

// Type returns the module type.
func (*Module) Type() string { return pluginID }

// Freeze is a no-op: the module has no mutable state.
func (*Module) Freeze() {}

// Truth makes a loaded module truthy.
func (*Module) Truth() starlark.Bool { return true }

// Hash rejects hashing a module.
func (*Module) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", pluginID) }

// Attr resolves the public operations.
func (m *Module) Attr(name string) (starlark.Value, error) {
	return star.Attr(m, name, builtins, nil)
}

// AttrNames returns the public operation names.
func (*Module) AttrNames() []string { return star.AttrNames(builtins, nil) }
