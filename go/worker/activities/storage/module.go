package storage

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/minio"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

// Module defines the dependency injection options for the fx framework.
// It provides YARPC clients for the RayClusterService and RayJobService,
// and registers the necessary activities with the worker.
var Module = fx.Options(
	minio.Module,
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the activities for Ray cluster and job operations to the provided workers.
func register(workers []worker.Worker,
	storages []Storage) {

	storageMap := make(map[string]Storage, len(storages))
	for _, storage := range storages {
		storageMap[storage.Protocol()] = storage
	}

	// Initialize the activities struct with the YARPC clients for Ray services.
	a := &activities{
		impls: storageMap,
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
