package handler

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BlobHandlerImpl implements the BlobHandler interface by delegating to the
// configured blob storage backend. This provides a consistent abstraction layer
// for blob operations while supporting different storage implementations.
type BlobHandlerImpl struct {
	storage storage.BlobStorage
}

// NewBlobHandler creates a new BlobHandler implementation.
// If storage is nil, returns a NullBlobHandler that provides safe no-op behavior.
func NewBlobHandler(storage storage.BlobStorage) BlobHandler {
	if storage == nil {
		return &NullBlobHandler{}
	}
	return &BlobHandlerImpl{storage: storage}
}

// IsObjectInteresting implements BlobHandler.IsObjectInteresting by delegating to the storage backend.
func (b *BlobHandlerImpl) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	return b.storage.IsObjectInteresting(obj)
}

// MergeWithExternalBlob implements BlobHandler.MergeWithExternalBlob by delegating to the storage backend.
func (b *BlobHandlerImpl) MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	return b.storage.MergeWithExternalBlob(ctx, obj)
}

// DeleteFromBlobStorage implements BlobHandler.DeleteFromBlobStorage by delegating to the storage backend.
func (b *BlobHandlerImpl) DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error {
	return b.storage.DeleteFromBlobStorage(ctx, obj)
}

// NullBlobHandler provides a safe no-op implementation of BlobHandler.
// This is used when blob storage is disabled, ensuring that the system
// continues to function while gracefully handling the absence of blob storage.
type NullBlobHandler struct{}

// IsObjectInteresting implements BlobHandler.IsObjectInteresting by always returning false.
// When blob storage is disabled, no objects are considered interesting for blob storage.
func (n *NullBlobHandler) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	return false
}

// MergeWithExternalBlob implements BlobHandler.MergeWithExternalBlob as a no-op when blob storage is disabled.
func (n *NullBlobHandler) MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

// DeleteFromBlobStorage implements BlobHandler.DeleteFromBlobStorage as a no-op when blob storage is disabled.
func (n *NullBlobHandler) DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}