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

// GetFromMetadata implements MetadataHandler.GetFromMetadata by delegating to the storage backend.
func (m *MetadataHandlerImpl) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return m.storage.GetByName(ctx, namespace, name, obj)
}

// UpdateInMetadata implements MetadataHandler.UpdateInMetadata by delegating to the handleUpdate function.
func (m *MetadataHandlerImpl) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return handleUpdate(ctx, obj, m.storage, true, nil, m.blobStorage)
}

// DeleteFromMetadata implements MetadataHandler.DeleteFromMetadata by delegating to the handleDelete function.
func (m *MetadataHandlerImpl) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	typeMeta, err := getObjectTypeMeta(obj)
	if err != nil {
		return err
	}
	return handleDelete(ctx, m.logger, typeMeta, obj, m.storage, m.blobStorage)
}

// ListFromMetadata implements MetadataHandler.ListFromMetadata by delegating to the storage backend.
func (m *MetadataHandlerImpl) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
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

// GetFromMetadata implements MetadataHandler.GetFromMetadata by returning a NotFound error.
// This maintains API consistency when metadata storage is disabled.
func (n *NullMetadataHandler) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, name)
}

// UpdateInMetadata implements MetadataHandler.UpdateInMetadata as a no-op when metadata storage is disabled.
func (n *NullMetadataHandler) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

// DeleteFromMetadata implements MetadataHandler.DeleteFromMetadata as a no-op when metadata storage is disabled.
func (n *NullMetadataHandler) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

// ListFromMetadata implements MetadataHandler.ListFromMetadata by returning a NotFound error.
// This maintains API consistency when metadata storage is disabled.
func (n *NullMetadataHandler) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, "")
}
