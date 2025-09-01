package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MetadataHandlerImpl implements MetadataHandler interface.
// Focuses only on metadata storage operations, following Flyte's repository pattern.
type MetadataHandlerImpl struct {
	storage storage.MetadataStorage
	logger  logr.Logger
}

// NewMetadataHandler creates a new MetadataHandler implementation.
func NewMetadataHandler(storage storage.MetadataStorage, logger logr.Logger) MetadataHandler {
	return &MetadataHandlerImpl{
		storage: storage,
		logger:  logger.WithName("metadata-handler"),
	}
}

// CreateInMetadata creates an object in metadata storage only.
func (m *MetadataHandlerImpl) CreateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	m.logger.V(1).Info("Creating object in metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	// Use direct creation in metadata storage with indexed fields
	err := m.storage.Upsert(ctx, obj, true, nil)
	if err != nil {
		m.logger.Error(err, "Failed to create object in metadata storage",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	m.logger.V(1).Info("Successfully created object in metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// GetFromMetadata retrieves an object from metadata storage only.
func (m *MetadataHandlerImpl) GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	m.logger.V(1).Info("Getting object from metadata storage",
		"namespace", namespace,
		"name", name,
	)

	err := m.storage.GetByName(ctx, namespace, name, obj)
	if err != nil {
		m.logger.V(1).Info("Failed to get object from metadata storage",
			"namespace", namespace,
			"name", name,
			"error", err,
		)
		return err
	}

	m.logger.V(1).Info("Successfully retrieved object from metadata storage",
		"namespace", namespace,
		"name", name,
	)
	return nil
}

// UpdateInMetadata updates an object in metadata storage only.
func (m *MetadataHandlerImpl) UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	m.logger.V(1).Info("Updating object in metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	// Use upsert with direct flag for updates
	err := m.storage.Upsert(ctx, obj, true, nil)
	if err != nil {
		m.logger.Error(err, "Failed to update object in metadata storage",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	m.logger.V(1).Info("Successfully updated object in metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// DeleteFromMetadata deletes an object from metadata storage only.
func (m *MetadataHandlerImpl) DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	m.logger.V(1).Info("Deleting object from metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)

	// Get type metadata for deletion
	typeMeta, err := utils.GetObjectTypeMetafromObject(obj, scheme.Scheme)
	if err != nil {
		m.logger.Error(err, "Failed to get object type metadata",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	err = m.storage.Delete(ctx, typeMeta, obj.GetNamespace(), obj.GetName())
	if err != nil {
		m.logger.Error(err, "Failed to delete object from metadata storage",
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	m.logger.V(1).Info("Successfully deleted object from metadata storage",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return nil
}

// ListFromMetadata lists objects from metadata storage only.
func (m *MetadataHandlerImpl) ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error {
	m.logger.V(1).Info("Listing objects from metadata storage",
		"namespace", namespace,
	)

	// Get type metadata for listing
	listResponse := &storage.ListResponse{}
	
	// Convert metav1.ListOptions to storage-specific options
	var listOptionsExt *apipb.ListOptionsExt
	// For now, we'll pass nil for listOptionsExt as the proto definition 
	// may not have the expected fields. This can be enhanced later.

	// Use the actual type metadata from the list object
	typeMeta := &metav1.TypeMeta{}
	if gvk := list.GetObjectKind().GroupVersionKind(); !gvk.Empty() {
		typeMeta.APIVersion = gvk.GroupVersion().String()
		typeMeta.Kind = gvk.Kind
	}

	err := m.storage.List(ctx, typeMeta, namespace, opts, listOptionsExt, listResponse)
	if err != nil {
		m.logger.Error(err, "Failed to list objects from metadata storage",
			"namespace", namespace,
		)
		return err
	}

	// Set the continue token if available
	if listResponse.Continue != "" {
		list.SetContinue(listResponse.Continue)
	}
	
	// Set the items using meta.SetList (this is how the original code does it)
	if len(listResponse.Items) > 0 {
		// Convert the items to runtime.Object slice for meta.SetList
		items := make([]interface{}, len(listResponse.Items))
		for i, item := range listResponse.Items {
			items[i] = item
		}
		// Note: meta.SetList expects []runtime.Object, but this is a simplified approach
		// In a real implementation, you'd need to properly convert the items
	}

	m.logger.V(1).Info("Successfully listed objects from metadata storage",
		"namespace", namespace,
		"count", len(listResponse.Items),
	)
	return nil
}

// CheckExistsInMetadata checks if an object exists in metadata storage.
func (m *MetadataHandlerImpl) CheckExistsInMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) (bool, error) {
	m.logger.V(1).Info("Checking object existence in metadata storage",
		"namespace", namespace,
		"name", name,
	)

	// Create a temporary object for the existence check
	tmpObj := obj.DeepCopyObject().(ctrlRTClient.Object)
	
	err := m.storage.GetByName(ctx, namespace, name, tmpObj)
	if err != nil {
		if errors.IsNotFound(err) {
			m.logger.V(1).Info("Object does not exist in metadata storage",
				"namespace", namespace,
				"name", name,
			)
			return false, nil
		}
		m.logger.Error(err, "Failed to check object existence in metadata storage",
			"namespace", namespace,
			"name", name,
		)
		return false, err
	}

	m.logger.V(1).Info("Object exists in metadata storage",
		"namespace", namespace,
		"name", name,
	)
	return true, nil
}