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

// LimitedGetter stops reading a blob after a byte limit, so that a large blob
// never fully loads into memory.
type LimitedGetter interface {
	// GetLimited returns at most limit+1 bytes of the blob.
	GetLimited(ctx context.Context, blobURI string, limit int64) ([]byte, error)
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
	client, err := b.clientFor(blobURI)
	if err != nil {
		return nil, err
	}
	return client.Get(ctx, blobURI)
}

// GetLimited returns at most limit+1 bytes of a blob. A result longer than
// limit tells the caller that the blob is larger than limit. A client that is
// not a LimitedGetter reads the whole blob, and GetLimited then truncates it.
func (b *BlobStore) GetLimited(ctx context.Context, blobURI string, limit int64) ([]byte, error) {
	client, err := b.clientFor(blobURI)
	if err != nil {
		return nil, err
	}
	if lg, ok := client.(LimitedGetter); ok {
		return lg.GetLimited(ctx, blobURI, limit)
	}
	data, err := client.Get(ctx, blobURI)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		data = data[:limit+1]
	}
	return data, nil
}

func (b *BlobStore) clientFor(blobURI string) (BlobStoreClient, error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	client, err := b.GetClient(parsedURL.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return client, nil
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
