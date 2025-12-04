package deployment

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for Deployment operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the DeploymentService and RevisionService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Deployment operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - deploymentService (v2pb.DeploymentServiceYARPCClient): YARPC client for Deployment operations.
// - revisionService (v2pb.RevisionServiceYARPCClient): YARPC client for Revision operations.
func register(workers []worker.Worker,
	deploymentService v2pb.DeploymentServiceYARPCClient,
	revisionService v2pb.RevisionServiceYARPCClient,
) {
	// Initialize the activities struct with the YARPC clients for Deployment services.
	a := &activities{
		deploymentService: deploymentService,
		revisionService:   revisionService,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
