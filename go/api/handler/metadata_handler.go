package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MetadataHandlerImpl implements the MetadataHandler interface by delegating to the
// configured metadata storage backend. This provides a consistent abstraction layer
// for metadata operations while supporting different storage implementations.
type MetadataHandlerImpl struct {
	storage     storage.MetadataStorage
	blobStorage storage.BlobStorage
	logger      logr.Logger
}

// NewMetadataHandler creates a new MetadataHandler implementation.
// If storage is nil, returns a NullMetadataHandler that provides safe no-op behavior.
// The blobStorage and logger are used for integrated blob operations and observability.
func NewMetadataHandler(storage storage.MetadataStorage, blobStorage storage.BlobStorage, logger logr.Logger) MetadataHandler {
	if storage == nil {
		return &NullMetadataHandler{}
	}
	return &MetadataHandlerImpl{storage: storage, blobStorage: blobStorage, logger: logger}
}

// Get implements MetadataHandler.Get by delegating to the storage backend.
func (m *MetadataHandlerImpl) Get(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return m.storage.GetByName(ctx, namespace, name, obj)
}

// Update implements MetadataHandler.Update by delegating to the handleUpdate function.
func (m *MetadataHandlerImpl) Update(ctx context.Context, obj ctrlRTClient.Object) error {
	return handleUpdate(ctx, obj, m.storage, true, nil, m.blobStorage)
}

// Delete implements MetadataHandler.Delete by delegating to the handleDelete function.
func (m *MetadataHandlerImpl) Delete(ctx context.Context, obj ctrlRTClient.Object) error {
	typeMeta, err := getObjectTypeMeta(obj)
	if err != nil {
		return err
	}
	return handleDelete(ctx, m.logger, typeMeta, obj, m.storage, m.blobStorage)
}

// List implements MetadataHandler.List by delegating to the storage backend.
func (m *MetadataHandlerImpl) List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	listResponse := &storage.ListResponse{}
	typeMeta, err := getObjectTypeMetaFromList(list)
	if err != nil {
		return err
	}
	err = m.storage.List(ctx, typeMeta, namespace, opts, listOptionsExt, listResponse)
	if err != nil {
		return err
	}
	list.SetContinue(listResponse.Continue)
	return meta.SetList(list, listResponse.Items)
}

// getObjectTypeMeta extracts TypeMeta from a controller-runtime Object using the configured scheme.
func getObjectTypeMeta(obj ctrlRTClient.Object) (*metav1.TypeMeta, error) {
	return utils.GetObjectTypeMetafromObject(obj, scheme.Scheme)
}

// getObjectTypeMetaFromList extracts TypeMeta from a controller-runtime ObjectList using the configured scheme.
func getObjectTypeMetaFromList(list ctrlRTClient.ObjectList) (*metav1.TypeMeta, error) {
	return utils.GetObjectTypeMetaFromList(list, scheme.Scheme)
}

// NullMetadataHandler provides a safe no-op implementation of MetadataHandler.
// This is used when metadata storage is disabled, ensuring that the system
// continues to function while gracefully handling the absence of metadata storage.
type NullMetadataHandler struct{}

// Get implements MetadataHandler.Get by returning a NotFound error.
// This maintains API consistency when metadata storage is disabled.
func (n *NullMetadataHandler) Get(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, name)
}

// Update implements MetadataHandler.Update as a no-op when metadata storage is disabled.
func (n *NullMetadataHandler) Update(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

// Delete implements MetadataHandler.Delete as a no-op when metadata storage is disabled.
func (n *NullMetadataHandler) Delete(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

// List implements MetadataHandler.List by returning a NotFound error.
// This maintains API consistency when metadata storage is disabled.
func (n *NullMetadataHandler) List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, "")
}

// handleUpdate is a helper function for updating objects in metadata storage.
// This function handles the actual update operation by delegating to the storage layer.
func handleUpdate(ctx context.Context, obj ctrlRTClient.Object, metadataStorage storage.MetadataStorage, direct bool,
	indexedFields []storage.IndexedField, handler storage.BlobStorage) error {
	// TODO(#555): update the object in blob storage
	return metadataStorage.Upsert(ctx, obj, direct, indexedFields)
}

// handleDelete is a helper function for deleting objects from metadata storage and blob storage.
// 1. Gets the object currently stored in metadataStorage, to retrieve the annotations
// 2. Deletes the object in metadataStorage
// 3. Deletes the object in blob storage
func handleDelete(ctx context.Context, log logr.Logger, typeMeta *metav1.TypeMeta, object ctrlRTClient.Object,
	metadataStorage storage.MetadataStorage, handler storage.BlobStorage) error {
	if handler.IsObjectInteresting(object) {
		// TODO(#556): if blob annotations are already available, this Get is not needed
		getErr := metadataStorage.GetByID(ctx, string(object.GetUID()), object)
		if err := metadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName()); err != nil {
			return err
		}

		if getErr == nil {
			// Failed to delete in blob storage is not a critical failure, as orphan blobs can be deleted by garbage
			// collector. So, do not return error.
			err := handler.DeleteFromBlobStorage(ctx, object)
			log.Error(err, "Failed to delete object in blob storage", "uid", object.GetUID())
		}

		return nil
	}

	return metadataStorage.Delete(ctx, typeMeta, object.GetNamespace(), object.GetName())
}
