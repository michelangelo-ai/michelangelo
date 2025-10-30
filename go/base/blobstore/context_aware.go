package blobstore

import (
	"context"
	"go.uber.org/zap"
)

// ContextKey is the type used for context keys to avoid collisions
type ContextKey string

const (
	// StorageProviderKey is the context key for storing the storage provider
	StorageProviderKey ContextKey = "storage_provider"
)

// WithStorageProvider adds a storage provider to the context
func WithStorageProvider(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, StorageProviderKey, provider)
}

// GetStorageProvider extracts the storage provider from context
func GetStorageProvider(ctx context.Context) string {
	if provider, ok := ctx.Value(StorageProviderKey).(string); ok {
		return provider
	}
	return ""
}

// ContextAwareBlobStore is an abstraction layer that automatically
// routes blob operations based on the storage provider in the context
type ContextAwareBlobStore struct {
	blobStore *BlobStore
	logger    *zap.Logger
}

// NewContextAwareBlobStore creates a new context-aware blob store
func NewContextAwareBlobStore(blobStore *BlobStore, logger *zap.Logger) *ContextAwareBlobStore {
	return &ContextAwareBlobStore{
		blobStore: blobStore,
		logger:    logger.With(zap.String("component", "context-aware-blobstore")),
	}
}

// Get retrieves blob content, automatically using the storage provider from context
func (c *ContextAwareBlobStore) Get(ctx context.Context, blobURI string) ([]byte, error) {
	provider := GetStorageProvider(ctx)

	if provider != "" {
		c.logger.Debug("Using storage provider for blob retrieval",
			zap.String("storage_provider", provider),
			zap.String("blob_uri", blobURI))
		return c.blobStore.GetWithProvider(ctx, blobURI, provider)
	}

	c.logger.Debug("Using scheme-based routing for blob retrieval (no storage provider)",
		zap.String("blob_uri", blobURI))
	return c.blobStore.Get(ctx, blobURI)
}

// TODO: Add Put, Delete, Exists methods when BlobStore supports them with providers