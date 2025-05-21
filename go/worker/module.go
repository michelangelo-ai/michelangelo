package worker

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities"
	"github.com/michelangelo-ai/michelangelo/go/worker/starlark"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows"
	"go.uber.org/fx"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// Module provides YARPC client instances.
var Module = fx.Options(
	blobstore.Module,
	fx.Provide(NewConfig, NewYARPCDispatcher),
	fx.Provide(
		NewRayClusterServiceClient,
		NewRayJobServiceClient,
		NewSparkJobServiceClient,
		NewCachedOutputServiceClient,
	),
	workflowfx.Module,
	activities.Module,
	workflows.Module,
	starlark.Module,
)
