package storage

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Define an fx.In struct to receive the group.
type storagesIn struct {
	fx.In
	Workers   []worker.Worker
	BlobStore *blobstore.BlobStore
	Logger    *zap.Logger
}

// Module provides the fx dependency injection options,
// including the MinIO module and activity registration.
var Module = fx.Options(
	fx.Invoke(register), // Register storage activities with Cadence workers.
)

// register maps Storage implementations by protocol and registers
// the resulting activities with each Cadence worker.
func register(in storagesIn) {
	// Use regular blob store with explicit provider parameters
	a := &activities{
		blobStore: in.BlobStore,
		logger:    in.Logger,
	}

	for _, w := range in.Workers {
		w.RegisterActivity(a)
	}
}
