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
	Get(ctx context.Context, blobURI string) (any, error)
	// Scheme returns the scheme of the blob store client. For example, "s3" or "gs".
	Scheme() string
}

// BlobStore is a wrapper around a map of BlobStoreClient implementations.
type BlobStore struct {
	logger  *zap.Logger
	clients map[string]BlobStoreClient
}

// Get retrieves the content of a blob from the blob store.
// The blobURI is expected to be in the format "scheme://host(optional)/path".
// It parses the URL to extract the scheme, host, and path, then delegates to the appropriate client.
func (b *BlobStore) Get(ctx context.Context, blobURI string) (any, error) {
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
	client, ok := b.clients[scheme]
	if !ok {
		return nil, fmt.Errorf("scheme %s is not supported", scheme)
	}
	return client, nil
}

func (b *BlobStore) RegisterClient(client BlobStoreClient) {
	if b.clients == nil {
		b.clients = make(map[string]BlobStoreClient)
	}
	b.clients[client.Scheme()] = client
}
