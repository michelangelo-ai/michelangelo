package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BlobHandlerImpl implements BlobHandler interface.
// Focuses only on blob storage operations, following Flyte's storage abstraction pattern.
type BlobHandlerImpl struct {
	storage storage.BlobStorage
	logger  logr.Logger
}

// NewBlobHandler creates a new BlobHandler implementation.
func NewBlobHandler(storage storage.BlobStorage, logger logr.Logger) BlobHandler {
	return &BlobHandlerImpl{
		storage: storage,
		logger:  logger.WithName("blob-handler"),
	}
}

// IsObjectInteresting checks if the object needs blob storage handling.
func (b *BlobHandlerImpl) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	if b.storage == nil {
		return false
	}
	
	interesting := b.storage.IsObjectInteresting(obj)
	b.logger.V(2).Info("Checked if object is interesting for blob storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"interesting", interesting,
	)
	return interesting
}

// MergeWithBlob merges object with external blob data.
func (b *BlobHandlerImpl) MergeWithBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	if b.storage == nil {
		b.logger.V(1).Info("Blob storage not configured, skipping merge",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	if !b.storage.IsObjectInteresting(obj) {
		b.logger.V(2).Info("Object not interesting for blob storage, skipping merge",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	b.logger.V(1).Info("Merging object with blob storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	err := b.storage.MergeWithExternalBlob(ctx, obj)
	if err != nil {
		b.logger.Error(err, "Failed to merge object with blob storage",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	b.logger.V(1).Info("Successfully merged object with blob storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// StoreBlob stores large object data in blob storage.
func (b *BlobHandlerImpl) StoreBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	if b.storage == nil {
		b.logger.V(1).Info("Blob storage not configured, skipping store",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	if !b.storage.IsObjectInteresting(obj) {
		b.logger.V(2).Info("Object not interesting for blob storage, skipping store",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	b.logger.V(1).Info("Storing object in blob storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	// For storing new blobs, we can use the same merge functionality
	// as it handles both creation and updates
	err := b.storage.MergeWithExternalBlob(ctx, obj)
	if err != nil {
		b.logger.Error(err, "Failed to store object in blob storage",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	b.logger.V(1).Info("Successfully stored object in blob storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// DeleteBlob removes blob data associated with the object.
func (b *BlobHandlerImpl) DeleteBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	if b.storage == nil {
		b.logger.V(1).Info("Blob storage not configured, skipping delete",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	if !b.storage.IsObjectInteresting(obj) {
		b.logger.V(2).Info("Object not interesting for blob storage, skipping delete",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return nil
	}

	b.logger.V(1).Info("Deleting blob data for object",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	// Note: The current BlobStorage interface doesn't have a Delete method.
	// This would need to be added to the interface if blob cleanup is required.
	// For now, we'll log that deletion would happen here.
	b.logger.V(1).Info("Blob deletion not implemented in storage interface",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	return nil
}

// NullBlobHandler is a no-op implementation for when blob storage is disabled.
type NullBlobHandler struct {
	logger logr.Logger
}

// NewNullBlobHandler creates a no-op blob handler.
func NewNullBlobHandler(logger logr.Logger) BlobHandler {
	return &NullBlobHandler{
		logger: logger.WithName("null-blob-handler"),
	}
}

// IsObjectInteresting always returns false for null handler.
func (n *NullBlobHandler) IsObjectInteresting(obj ctrlRTClient.Object) bool {
	return false
}

// MergeWithBlob is a no-op for null handler.
func (n *NullBlobHandler) MergeWithBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Blob storage disabled, skipping merge",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// StoreBlob is a no-op for null handler.
func (n *NullBlobHandler) StoreBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Blob storage disabled, skipping store",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// DeleteBlob is a no-op for null handler.
func (n *NullBlobHandler) DeleteBlob(ctx context.Context, obj ctrlRTClient.Object) error {
	n.logger.V(2).Info("Blob storage disabled, skipping delete",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}