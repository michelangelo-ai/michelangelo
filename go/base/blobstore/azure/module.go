package azure

import (
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"go.uber.org/fx"
)

type BlobStoreClientOut struct {
	fx.Out
	BlobStoreClient blobstore.BlobStoreClient `group:"blobstore_clients"`
}

// Module sets up dependency injection for the Azure Blob Storage client.
// It calls newConfig to initialize configuration and newClient to create the Azure client.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(newClient),
)

// newClient initializes a new azureBlobClient using the provided configuration.
// Required settings are validated on first use (see azureBlobClient.Get), not
// here, so providing this module never fails startup for deployments that do
// not configure Azure Blob Storage.
func newClient(config Config) BlobStoreClientOut {
	return BlobStoreClientOut{
		BlobStoreClient: newAzureBlobClient(config.StorageAccount, config.SASToken, config.Endpoint),
	}
}
