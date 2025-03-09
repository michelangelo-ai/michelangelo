package ray

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
)

const serverName = "ma-apiserver" // The name of the API server providing YARPC services for Ray operations.

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the RayClusterService and RayJobService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Ray cluster and job operations to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Temporal workers where activities will be registered.
// - rayJob (v2pb.RayJobServiceClient): gRPC client for Ray job operations.
// - rayCluster (v2pb.RayClusterServiceClient): gRPC client for Ray cluster operations.
func register(w worker.Worker,
	rayJob v2pb.RayJobServiceYARPCClient,
	rayCluster v2pb.RayClusterServiceYARPCClient) {

	a := &activities{
		rayClusterService: rayCluster,
		rayJobService:     rayJob,
	}
	w.RegisterActivity(a) // Register Ray cluster service activities
}
