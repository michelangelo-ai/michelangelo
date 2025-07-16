package worker

import (
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore/minio"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/rayhttp"
	rayhttpPlugin "github.com/michelangelo-ai/michelangelo/go/worker/plugins/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/worker/starlark"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows"
	"go.uber.org/fx"
)

// Module provides YARPC client instances.
var Module = fx.Options(
	fx.Provide(NewConfig, NewYARPCDispatcher),
	fx.Provide(
		NewRayClusterServiceClient,
		NewRayJobServiceClient,
		NewSparkJobServiceClient,
		NewCachedOutputServiceClient,
		GetRayHTTPConfig,
	),
	workflowfx.Module,
	activities.Module,
	rayhttp.Module,
	workflows.Module,
	starlark.Module,
	blobstore.Module,
	minio.Module,
	fx.Provide(
		func() *rayhttpPlugin.Plugin {
			return rayhttpPlugin.Plugin
		},
	),
)
