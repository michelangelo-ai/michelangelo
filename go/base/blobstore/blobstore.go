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
	// Put uploads data to the given URI in the blob store.
	Put(ctx context.Context, blobURI string, data []byte) error
	// Delete removes the blob at the given URI from the blob store.
	Delete(ctx context.Context, blobURI string) error
	// Scheme returns the scheme of the blob store client. For example, "s3" or "gs".
	Scheme() string
}

// BlobStore is a wrapper around a map of BlobStoreClient implementations.
type BlobStore struct {
	Logger  *zap.Logger
	Clients map[string]BlobStoreClient
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

// Put uploads data to the given URI, delegating to the matching scheme client.
func (b *BlobStore) Put(ctx context.Context, blobURI string, data []byte) error {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}
	client, err := b.GetClient(parsedURL.Scheme)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	return client.Put(ctx, blobURI, data)
}

// Delete removes the blob at the given URI, delegating to the matching scheme client.
func (b *BlobStore) Delete(ctx context.Context, blobURI string) error {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}
	client, err := b.GetClient(parsedURL.Scheme)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	return client.Delete(ctx, blobURI)
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
	b.Clients[client.Scheme()] = client
}
