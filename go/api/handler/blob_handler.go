package handler

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BlobHandlerImpl wraps existing blobStorage functionality with NO new logic
type BlobHandlerImpl struct {
	storage storage.BlobStorage
}

func NewBlobHandler(storage storage.BlobStorage) BlobHandler {
	if storage == nil {
		return &NullBlobHandler{}
	}
	return &BlobHandlerImpl{storage: storage}
}

// IsObjectInteresting directly delegates to existing blobStorage.IsObjectInteresting - NO new logic
func (b *BlobHandlerImpl) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	return b.storage.IsObjectInteresting(obj)
}

// MergeWithExternalBlob directly delegates to existing blobStorage.MergeWithExternalBlob - NO new logic
func (b *BlobHandlerImpl) MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	return b.storage.MergeWithExternalBlob(ctx, obj)
}

// DeleteFromBlobStorage directly delegates to existing blobStorage.DeleteFromBlobStorage - NO new logic
func (b *BlobHandlerImpl) DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error {
	return b.storage.DeleteFromBlobStorage(ctx, obj)
}

// NullBlobHandler provides empty implementation when blob storage is disabled
type NullBlobHandler struct{}

func (n *NullBlobHandler) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	return false
}

func (n *NullBlobHandler) MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

func (n *NullBlobHandler) DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}