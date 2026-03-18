package handler

import (
	"context"
	"fmt"

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

// PrepareForWrite uploads obj to blob storage if interesting, then returns a deep-copy with blob fields cleared.
func (b *BlobHandlerImpl) PrepareForWrite(ctx context.Context, obj ctrlRTClient.Object) (ctrlRTClient.Object, error) {
	fmt.Printf("[HANDLER] PrepareForWrite called: name=%q namespace=%q type=%T\n", obj.GetName(), obj.GetNamespace(), obj)
	if !b.storage.IsObjectInteresting(obj) {
		fmt.Printf("[HANDLER] PrepareForWrite: not interesting, skipping blob upload\n")
		return obj, nil
	}
	fmt.Printf("[HANDLER] PrepareForWrite: object is interesting, uploading to blob storage\n")
	if _, err := b.storage.UploadToBlobStorage(ctx, obj); err != nil {
		fmt.Printf("[HANDLER] PrepareForWrite: upload failed: %v\n", err)
		return nil, err
	}
	if blobFieldObj, ok := obj.(storage.ObjectWithBlobFields); ok && blobFieldObj.HasBlobFields() {
		copied := obj.DeepCopyObject().(ctrlRTClient.Object)
		copied.(storage.ObjectWithBlobFields).ClearBlobFields()
		fmt.Printf("[HANDLER] PrepareForWrite: blob fields cleared on deep copy\n")
		return copied, nil
	}
	fmt.Printf("[HANDLER] PrepareForWrite: no blob fields to clear, returning original\n")
	return obj, nil
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

// PrepareForWrite is a no-op when blob storage is disabled.
func (n *NullBlobHandler) PrepareForWrite(_ context.Context, obj ctrlRTClient.Object) (ctrlRTClient.Object, error) {
	fmt.Printf("[HANDLER] PrepareForWrite: NullBlobHandler (blob storage disabled), skipping\n")
	return obj, nil
}
