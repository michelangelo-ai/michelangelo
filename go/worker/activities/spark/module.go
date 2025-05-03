package spark

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for Spark operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the SparkClusterService and SparkJobService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Spark cluster and job operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - sparkJob (v2pb.SparkJobServiceYARPCClient): YARPC client for Spark job operations.
func register(workers []worker.Worker,
	rayJob v2pb.SparkJobServiceYARPCClient) {

	// Initialize the activities struct with the YARPC clients for Spark services.
	a := &activities{
		sparkJobService: rayJob,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
