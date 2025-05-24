package blobstore

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore/minio"
)

var Module = fx.Options(
	minio.Module,
	fx.Provide(NewBlobStore),
)

// Define an fx.In struct to receive the group.
type blobstoreClientsIn struct {
	fx.In
	BlobStoreClients []BlobStoreClient `group:"blobstore_clients"`
}

func NewBlobStore(in blobstoreClientsIn, logger *zap.Logger) *BlobStore {
	blobStore := BlobStore{
		clients: make(map[string]BlobStoreClient),
		logger:  logger.With(zap.String("component", "blobstore")),
	}
	for _, client := range in.BlobStoreClients {
		logger.Info("Registering blobstore client", zap.String("scheme", client.Scheme()))
		blobStore.RegisterClient(client)
	}
	return &blobStore
}
