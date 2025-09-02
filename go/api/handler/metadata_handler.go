package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MetadataHandlerImpl wraps existing metadataStorage functionality with NO new logic
type MetadataHandlerImpl struct {
	storage     storage.MetadataStorage
	blobStorage storage.BlobStorage
	logger      logr.Logger
}

func NewMetadataHandler(storage storage.MetadataStorage, blobStorage storage.BlobStorage, logger logr.Logger) MetadataHandler {
	if storage == nil {
		return &NullMetadataHandler{}
	}
	return &MetadataHandlerImpl{storage: storage, blobStorage: blobStorage, logger: logger}
}

// GetFromMetadata directly delegates to existing metadataStorage.GetByName - NO new logic
func (m *MetadataHandlerImpl) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return m.storage.GetByName(ctx, namespace, name, obj)
}


// UpdateInMetadata directly delegates to existing handleUpdate function - NO new logic
func (m *MetadataHandlerImpl) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return handleUpdate(ctx, obj, m.storage, true, nil, m.blobStorage)
}

// DeleteFromMetadata directly delegates to existing handleDelete function - NO new logic
func (m *MetadataHandlerImpl) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	typeMeta, err := getObjectTypeMeta(obj)
	if err != nil {
		return err
	}
	return handleDelete(ctx, m.logger, typeMeta, obj, m.storage, m.blobStorage)
}

// ListFromMetadata directly delegates to existing metadataStorage.List - NO new logic
func (m *MetadataHandlerImpl) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	listResponse := &storage.ListResponse{}
	typeMeta, err := getObjectTypeMetaFromList(list)
	if err != nil {
		return err
	}
	err = m.storage.List(ctx, typeMeta, namespace, opts, nil, listResponse)
	if err != nil {
		return err
	}
	list.SetContinue(listResponse.Continue)
	return meta.SetList(list, listResponse.Items)
}

// Helper function to get type meta - uses existing utility
func getObjectTypeMeta(obj ctrlRTClient.Object) (*metav1.TypeMeta, error) {
	return utils.GetObjectTypeMetafromObject(obj, scheme.Scheme)
}

// Helper function to get type meta from list - uses existing utility
func getObjectTypeMetaFromList(list ctrlRTClient.ObjectList) (*metav1.TypeMeta, error) {
	return utils.GetObjectTypeMetaFromList(list, scheme.Scheme)
}

// NullMetadataHandler provides empty implementation when metadata storage is disabled
type NullMetadataHandler struct{}

func (n *NullMetadataHandler) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, name)
}


func (n *NullMetadataHandler) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

func (n *NullMetadataHandler) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	return nil
}

func (n *NullMetadataHandler) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	return apiErrors.NewNotFound(schema.GroupResource{}, "")
}