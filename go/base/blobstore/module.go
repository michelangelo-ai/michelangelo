package blobstore

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Options(
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
		blobStore.RegisterClient(client)
	}
	return &blobStore
}
