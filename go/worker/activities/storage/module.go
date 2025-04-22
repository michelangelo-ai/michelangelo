package storage

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/minio"
)

// Define an fx.In struct to receive the group.
type storagesIn struct {
	fx.In
	Workers  []worker.Worker
	Storages []intf.Storage `group:"storages"`
}

// Module provides the fx dependency injection options,
// including the MinIO module and activity registration.
var Module = fx.Options(
	minio.Module,
	fx.Invoke(register), // Register storage activities with Cadence workers.
)

// register maps Storage implementations by protocol and registers
// the resulting activities with each Cadence worker.
func register(in storagesIn) {
	storageMap := make(map[string]intf.Storage, len(in.Storages))
	for _, storage := range in.Storages {
		storageMap[storage.Protocol()] = storage
	}

	a := &activities{
		impls: storageMap,
	}

	for _, w := range in.Workers {
		w.RegisterActivity(a)
	}
}
