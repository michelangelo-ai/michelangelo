package azure

import (
	"fmt"

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
// Returns a pointer to azureBlobClient or an error if initialization fails.
func newClient(config Config) (BlobStoreClientOut, error) {
	if config.StorageAccount == "" {
		return BlobStoreClientOut{}, fmt.Errorf("azure storage account is required")
	}
	if config.SASToken == "" {
		return BlobStoreClientOut{}, fmt.Errorf("azure SAS token is required")
	}

	azureClient := newAzureBlobClient(config.StorageAccount, config.SASToken, config.Endpoint)
	return BlobStoreClientOut{
		BlobStoreClient: azureClient,
	}, nil
}
