package model

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for Model operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the ModelService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Model operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - modelService (v2pb.ModelServiceYARPCClient): YARPC client for Model operations.
// - deploymentService (v2pb.DeploymentServiceYARPCClient): YARPC client for Deployment operations.
func register(workers []worker.Worker,
	modelService v2pb.ModelServiceYARPCClient,
	deploymentService v2pb.DeploymentServiceYARPCClient,
) {
	// Initialize the activities struct with the YARPC clients for Model services.
	a := &activities{
		modelService:      modelService,
		deploymentService: deploymentService,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
