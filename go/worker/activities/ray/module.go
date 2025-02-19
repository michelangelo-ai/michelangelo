package ray

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for Ray operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the RayClusterService and RayJobService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Provide(v2pb.NewFxRayClusterServiceYARPCClient(serverName)), // Provides the RayClusterService YARPC client.
	fx.Provide(v2pb.NewFxRayJobServiceYARPCClient(serverName)),     // Provides the RayJobService YARPC client.
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Ray cluster and job operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - rayJob (v2pb.RayJobServiceYARPCClient): YARPC client for Ray job operations.
// - rayCluster (v2pb.RayClusterServiceYARPCClient): YARPC client for Ray cluster operations.
func register(workers []worker.Worker,
	rayJob v2pb.RayJobServiceYARPCClient,
	rayCluster v2pb.RayClusterServiceYARPCClient) {

	// Initialize the activities struct with the YARPC clients for Ray services.
	a := &activities{
		rayClusterService: rayCluster,
		rayJobService:     rayJob,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
