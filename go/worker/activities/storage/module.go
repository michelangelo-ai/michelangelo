package storage

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/minio"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

// Module provides the fx dependency injection options,
// including the MinIO module and activity registration.
var Module = fx.Options(
	minio.Module,
	fx.Invoke(register), // Register storage activities with Cadence workers.
)

// register maps Storage implementations by protocol and registers
// the resulting activities with each Cadence worker.
func register(workers []worker.Worker, storages []Storage) {
	storageMap := make(map[string]Storage, len(storages))
	for _, storage := range storages {
		storageMap[storage.Protocol()] = storage
	}

	a := &activities{
		impls: storageMap,
	}

	for _, w := range workers {
		w.RegisterActivity(a)
	}
}
