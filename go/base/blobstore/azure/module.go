package azure

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

type BlobStoreClientOut struct {
	fx.Out
	BlobStoreClient blobstore.BlobStoreClient `group:"blobstore_clients"`
}

// Module sets up dependency injection for the Azure client.
// It calls newConfig to initialize configuration and newClient to create the Azure client.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(newClient),
)

// newClient initializes new Azure storage clients using the provided configuration.
// It creates clients for multiple Azure storage providers based on the configuration map.
// Returns multiple BlobStoreClientOut instances or an error if initialization fails.
func newClient(config Config) ([]BlobStoreClientOut, error) {
	var clients []BlobStoreClientOut

	// If no storage providers configured, return empty list (no default Azure client)
	if len(config.StorageProviders) == 0 {
		return clients, nil
	}

	// Create clients for each configured Azure storage provider
	for providerKey, providerConfig := range config.StorageProviders {
		if providerConfig.Type != "azure" {
			continue // Skip non-Azure providers
		}

		client, err := newAzureClientWithKey(providerKey, providerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client for provider %s: %w", providerKey, err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

// newAzureClientWithKey creates a new Azure Blob Storage client with provider key
func newAzureClientWithKey(providerKey string, config StorageProvider) (BlobStoreClientOut, error) {
	if config.AzureStorageAccount == "" {
		return BlobStoreClientOut{}, fmt.Errorf("azure storage account is required for provider %s", providerKey)
	}
	if config.AzureSASToken == "" {
		return BlobStoreClientOut{}, fmt.Errorf("azure SAS token is required for provider %s", providerKey)
	}

	azureClient := newAzureBlobClient(config.AzureStorageAccount, config.AzureSASToken, config.AzureEndpoint, providerKey)
	return BlobStoreClientOut{
		BlobStoreClient: azureClient,
	}, nil
}
