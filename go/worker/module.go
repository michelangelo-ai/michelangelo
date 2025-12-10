package worker

import (
	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/cadence-workflow/starlark-worker/service"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore/minio"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities"
	"github.com/michelangelo-ai/michelangelo/go/worker/starlark"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows"
)

// ProvidePluginRegistry creates a new plugin registry based on the global registry.
func ProvidePluginRegistry() map[string]service.IPlugin {
	// Start with the global plugin registry as base
	registry := make(map[string]service.IPlugin)
	for id, p := range plugin.Registry {
		registry[id] = p
	}
	return registry
}

// Module provides HTTP client instances for all HTTP-based activities.
var Module = fx.Options(
	fx.Provide(NewConfig, NewYARPCDispatcher),
	fx.Provide(
		NewRayClusterServiceClient,
		NewRayJobServiceClient,
		NewSparkJobServiceClient,
		NewCachedOutputServiceClient,
		NewPipelineRunServiceClient,
		NewModelServiceClient,
		NewDeploymentServiceClient,
		ProvidePluginRegistry,
	),
	workflowfx.Module,
	activities.Module,
	workflows.Module,
	starlark.Module,
	blobstore.Module,
	minio.Module,
)
