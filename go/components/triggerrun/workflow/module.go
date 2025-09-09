package workflow

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
)

// Module provides fx Options for workflow trigger activities.
var Module = fx.Options(
	fx.Invoke(register),
)

// register creates the workflow service and registers activities using RegisterActivityWithOptions.
func register(workers []worker.Worker, pipelineRunService v2pb.PipelineRunServiceYARPCClient) {
	// Create the service with the YARPC client
	service := &Service{
		PipelineRunService: pipelineRunService,
	}

	// Use the Register function with starlark workers (it will extract OSS workers internally)
	for _, w := range workers {
		Register(service, "trigger", w)
	}
}
