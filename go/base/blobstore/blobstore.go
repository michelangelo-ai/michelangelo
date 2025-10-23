package blobstore

import (
	"context"
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

// BlobStoreClient is an interface for a blob store client.
type BlobStoreClient interface {
	// Get retrieves the content of a blob from the blob store.
	// The blobURI is expected to be in the format "scheme://host(optional)/path".
	Get(ctx context.Context, blobURI string) ([]byte, error)
	// Scheme returns the scheme of the blob store client. For example, "s3" or "gs".
	Scheme() string
}

// ProviderClient extends BlobStoreClient with provider key support
type ProviderClient interface {
	BlobStoreClient
	// ProviderKey returns the provider key for this client
	ProviderKey() string
}

// BlobStore is a wrapper around a map of BlobStoreClient implementations.
type BlobStore struct {
	Logger          *zap.Logger
	Clients         map[string]BlobStoreClient // scheme -> client
	ProviderClients map[string]BlobStoreClient // provider_key -> client
}

// Get retrieves the content of a blob from the blob store.
// The blobURI is expected to be in the format "scheme://host(optional)/path".
// It parses the URL to extract the scheme, host, and path, then delegates to the appropriate client.
func (b *BlobStore) Get(ctx context.Context, blobURI string) ([]byte, error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	client, err := b.GetClient(parsedURL.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return client.Get(ctx, blobURI)
}

func (b *BlobStore) GetClient(scheme string) (BlobStoreClient, error) {
	client, ok := b.Clients[scheme]
	if !ok {
		return nil, fmt.Errorf("scheme %s is not supported", scheme)
	}
	return client, nil
}

func (b *BlobStore) RegisterClient(client BlobStoreClient) {
	if b.Clients == nil {
		b.Clients = make(map[string]BlobStoreClient)
	}
	if b.ProviderClients == nil {
		b.ProviderClients = make(map[string]BlobStoreClient)
	}

	// Register by scheme (existing functionality)
	b.Clients[client.Scheme()] = client

	// Register by provider key if client supports it
	if providerClient, ok := client.(ProviderClient); ok {
		b.ProviderClients[providerClient.ProviderKey()] = client
	}
}

// GetClientByProvider retrieves a client by provider key
func (b *BlobStore) GetClientByProvider(providerKey string) (BlobStoreClient, error) {
	client, ok := b.ProviderClients[providerKey]
	if !ok {
		return nil, fmt.Errorf("provider %s is not configured", providerKey)
	}
	return client, nil
}

// GetWithProvider retrieves blob content using a specific provider
func (b *BlobStore) GetWithProvider(ctx context.Context, blobURI string, providerKey string) ([]byte, error) {
	if providerKey != "" {
		client, err := b.GetClientByProvider(providerKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get client for provider %s: %w", providerKey, err)
		}
		return client.Get(ctx, blobURI)
	}

	// Fallback to scheme-based routing
	return b.Get(ctx, blobURI)
}
