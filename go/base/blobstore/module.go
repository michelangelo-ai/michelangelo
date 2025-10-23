package blobstore

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	Module = fx.Options(
		fx.Provide(NewBlobStore),
	)
)

// Define an fx.In struct to receive the group.
type blobstoreClientsIn struct {
	fx.In
	BlobStoreClients []BlobStoreClient `group:"blobstore_clients"`
}

func NewBlobStore(in blobstoreClientsIn, logger *zap.Logger) *BlobStore {
	blobStore := BlobStore{
		Clients:         make(map[string]BlobStoreClient),
		ProviderClients: make(map[string]BlobStoreClient),
		Logger:          logger.With(zap.String("component", "blobstore")),
	}
	for _, client := range in.BlobStoreClients {
		logger.Info("Registering blobstore client", zap.String("scheme", client.Scheme()))
		blobStore.RegisterClient(client)

		// Log provider key if available
		if providerClient, ok := client.(ProviderClient); ok {
			logger.Info("Registering blobstore provider",
				zap.String("provider_key", providerClient.ProviderKey()),
				zap.String("scheme", client.Scheme()))
		}
	}
	return &blobStore
}
