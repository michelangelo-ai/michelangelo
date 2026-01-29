package trigger

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/fx"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for trigger operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the PipelineRunService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for trigger operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - pipelineRunService (v2pb.PipelineRunServiceYARPCClient): YARPC client for pipeline run operations.
func register(workers []worker.Worker, pipelineRunService v2pb.PipelineRunServiceYARPCClient) {
	// Initialize the activities struct with the YARPC client for pipeline run services.
	a := &activities{
		pipelineRunService: pipelineRunService,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
