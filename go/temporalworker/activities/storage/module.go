package storage

import (
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"

	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/minio"
)

// Define an fx.In struct to receive the group.
type storagesIn struct {
	fx.In
	Worker   worker.Worker
	Storages []intf.Storage `group:"storages"`
}

// Module provides the fx dependency injection options,
// including the MinIO module and activity registration.
var Module = fx.Options(
	minio.Module,
	fx.Invoke(register), // Register storage activities with Temporal workers.
)

// register maps Storage implementations by protocol and registers
// the resulting activities with each Temporal worker.
func register(in storagesIn) {
	// Create a map to store storage implementations by protocol
	storageMap := make(map[string]intf.Storage, len(in.Storages))
	for _, storage := range in.Storages {
		storageMap[storage.Protocol()] = storage
	}

	// Create activities object with mapped storage implementations
	a := &activities{
		impls: storageMap,
	}

	// Register the activities with each Temporal worker
	in.Worker.RegisterActivity(a)
}
