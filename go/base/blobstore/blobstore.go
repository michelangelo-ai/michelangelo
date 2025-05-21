package blobstore

import (
	"context"
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

type BlobStoreClient interface {
	Get(ctx context.Context, uri string) (any, error)
	Scheme() string
}


type BlobStore struct {
	logger *zap.Logger
	clients map[string]BlobStoreClient
}

func (b *BlobStore) Get(ctx context.Context, uri string) (any, error) {
	parsedUri, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uri: %w", err)
	}
	client, err := b.GetClient(parsedUri.Scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}
	return client.Get(ctx, parsedUri.Path)
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




