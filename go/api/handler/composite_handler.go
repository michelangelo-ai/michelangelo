package handler

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlRTApiutil "sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrlRTUtil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CompositeAPIHandler implements api.Handler by orchestrating between focused handlers.
// Inspired by Flyte's manager pattern and Kubeflow's resource manager approach.
type CompositeAPIHandler struct {
	// Focused handlers for each concern
	k8s        K8sHandler
	metadata   MetadataHandler
	blob       BlobHandler
	validation ValidationHandler
	metrics    MetricsHandler

	// Configuration
	config *Config
	logger logr.Logger
}

// NewCompositeAPIHandler creates a new composite API handler.
func NewCompositeAPIHandler(
	k8s K8sHandler,
	metadata MetadataHandler,
	blob BlobHandler,
	validation ValidationHandler,
	metrics MetricsHandler,
	config *Config,
	logger logr.Logger,
) api.Handler {
	return &CompositeAPIHandler{
		k8s:        k8s,
		metadata:   metadata,
		blob:       blob,
		validation: validation,
		metrics:    metrics,
		config:     config,
		logger:     logger.WithName("composite-handler"),
	}
}

// Create implements api.Handler.Create following Flyte's concurrent operation pattern.
func (c *CompositeAPIHandler) Create(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	start := time.Now()
	operation := "Create"
	kind := c.getObjectKind(obj)
	
	c.logger.Info("Starting create operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", kind,
	)

	// Step 1: Validation (following Flyte's validation-first pattern)
	if err := c.validation.ValidateCreate(obj); err != nil {
		c.recordError(operation, "validation_failed", kind)
		return c.wrapError(err, "validation failed", obj)
	}

	// Step 2: Check existence in metadata storage if enabled
	if c.config.EnableMetadataStorage && c.metadata != nil {
		exists, err := c.metadata.CheckExistsInMetadata(ctx, obj.GetNamespace(), obj.GetName(), obj)
		if err != nil {
			c.recordError(operation, "metadata_check_failed", kind)
			return c.wrapError(err, "failed to check existence in metadata storage", obj)
		}
		if exists {
			c.recordError(operation, "already_exists", kind)
			return status.Errorf(codes.AlreadyExists, 
				"object already exists: namespace=%s, name=%s", 
				obj.GetNamespace(), obj.GetName())
		}

		// Add ingester finalizer for metadata storage
		objMeta, err := meta.Accessor(obj)
		if err != nil {
			c.recordError(operation, "metadata_accessor_failed", kind)
			return c.wrapError(err, "failed to access object metadata", obj)
		}
		ctrlRTUtil.AddFinalizer(objMeta.(ctrlRTClient.Object), api.IngesterFinalizer)
	}

	// Step 3: Set update timestamp
	c.setUpdateTimestamp(obj, true)

	// Step 4: Create in K8s (primary storage)
	if err := c.k8s.CreateInK8s(ctx, obj, opts); err != nil {
		c.recordError(operation, "k8s_create_failed", kind)
		return c.wrapError(err, "failed to create in K8s", obj)
	}

	// Step 5: Handle blob storage if needed (following Flyte's storage pattern)
	if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
		if err := c.blob.StoreBlob(ctx, obj); err != nil {
			c.logger.Error(err, "Failed to store blob, but K8s object created",
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
			// Don't fail the operation if blob storage fails
		}
	}

	// Record success metrics
	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.Info("Successfully completed create operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"duration", time.Since(start),
	)

	return nil
}

// Get implements api.Handler.Get following Flyte's storage retrieval pattern.
func (c *CompositeAPIHandler) Get(ctx context.Context, namespace string, name string, opts *metav1.GetOptions, obj ctrlRTClient.Object) error {
	start := time.Now()
	operation := "Get"
	kind := c.getObjectKind(obj)

	c.logger.V(1).Info("Starting get operation",
		"namespace", namespace,
		"name", name,
		"kind", kind,
	)

	// Step 1: Try to get from K8s first
	err := c.k8s.GetFromK8s(ctx, namespace, name, obj)
	if err == nil && !c.isDeletedImmutableObject(obj) {
		// Step 2: Merge with blob storage if needed
		if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
			if blobErr := c.blob.MergeWithBlob(ctx, obj); blobErr != nil {
				c.logger.Error(blobErr, "Failed to merge with blob storage",
					"namespace", namespace,
					"name", name,
				)
				// Don't fail the operation if blob merge fails
			}
		}

		c.recordSuccess(operation, time.Since(start), kind)
		c.logger.V(1).Info("Successfully retrieved object from K8s",
			"namespace", namespace,
			"name", name,
		)
		return nil
	}

	// Step 3: If not found in K8s and not a simple NotFound error, return error
	if !apiErrors.IsNotFound(err) {
		c.recordError(operation, "k8s_get_failed", kind)
		return c.wrapError(err, "failed to get from K8s", obj)
	}

	// Step 4: Fallback to metadata storage if enabled
	if c.config.EnableMetadataStorage && c.metadata != nil {
		if metaErr := c.metadata.GetFromMetadata(ctx, namespace, name, obj); metaErr != nil {
			c.recordError(operation, "not_found", kind)
			return c.wrapError(metaErr, "object not found", obj)
		}

		// Step 5: Merge with blob storage if needed
		if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
			if blobErr := c.blob.MergeWithBlob(ctx, obj); blobErr != nil {
				c.logger.Error(blobErr, "Failed to merge with blob storage",
					"namespace", namespace,
					"name", name,
				)
				// Don't fail the operation if blob merge fails
			}
		}

		c.recordSuccess(operation, time.Since(start), kind)
		c.logger.V(1).Info("Successfully retrieved object from metadata storage",
			"namespace", namespace,
			"name", name,
		)
		return nil
	}

	c.recordError(operation, "not_found", kind)
	return c.wrapError(err, "object not found", obj)
}

// Update implements api.Handler.Update following Flyte's update pattern.
func (c *CompositeAPIHandler) Update(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	start := time.Now()
	operation := "Update"
	kind := c.getObjectKind(obj)

	c.logger.Info("Starting update operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", kind,
	)

	// Step 1: Validation
	if err := c.validation.ValidateUpdate(obj); err != nil {
		c.recordError(operation, "validation_failed", kind)
		return c.wrapError(err, "validation failed", obj)
	}

	// Step 2: Check for spec changes
	hasSpecChange, err := c.hasSpecChange(ctx, obj)
	if err != nil {
		c.recordError(operation, "spec_check_failed", kind)
		return c.wrapError(err, "failed to check spec changes", obj)
	}

	// Step 3: Set update timestamp
	c.setUpdateTimestamp(obj, hasSpecChange)

	// Step 4: Create a copy for fallback handling
	tmpObj, ok := obj.DeepCopyObject().(ctrlRTClient.Object)
	if !ok {
		c.recordError(operation, "copy_failed", kind)
		return status.Errorf(codes.Internal, "object does not implement controller-runtime client.Object interface")
	}

	// Step 5: Try to update in K8s
	err = c.k8s.UpdateInK8s(ctx, obj, opts)
	
	// Step 6: If object not found in K8s, update in metadata storage directly
	if apiErrors.IsNotFound(err) && c.config.EnableMetadataStorage && c.metadata != nil {
		if updateErr := c.handleUpdateInMetadata(ctx, tmpObj); updateErr != nil {
			c.recordError(operation, "metadata_update_failed", kind)
			return c.wrapError(updateErr, "failed to update in metadata storage", tmpObj)
		}
	} else if err != nil {
		c.recordError(operation, "k8s_update_failed", kind)
		return c.wrapError(err, "failed to update in K8s", obj)
	}

	// Step 7: Handle blob storage if needed
	if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
		if blobErr := c.blob.StoreBlob(ctx, obj); blobErr != nil {
			c.logger.Error(blobErr, "Failed to update blob storage",
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
			// Don't fail the operation if blob storage fails
		}
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.Info("Successfully completed update operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"duration", time.Since(start),
	)

	return nil
}

// UpdateStatus implements api.Handler.UpdateStatus.
func (c *CompositeAPIHandler) UpdateStatus(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	start := time.Now()
	operation := "UpdateStatus"
	kind := c.getObjectKind(obj)

	c.logger.V(1).Info("Starting update status operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", kind,
	)

	// Status updates primarily go to K8s
	err := c.k8s.UpdateStatusInK8s(ctx, obj, opts)
	if err != nil {
		c.recordError(operation, "k8s_status_update_failed", kind)
		return c.wrapError(err, "failed to update status in K8s", obj)
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.V(1).Info("Successfully completed update status operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"duration", time.Since(start),
	)

	return nil
}

// Delete implements api.Handler.Delete following Flyte's deletion pattern.
func (c *CompositeAPIHandler) Delete(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error {
	start := time.Now()
	operation := "Delete"
	kind := c.getObjectKind(obj)

	c.logger.Info("Starting delete operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"kind", kind,
	)

	// Step 1: Validation
	if err := c.validation.ValidateDelete(obj); err != nil {
		c.recordError(operation, "validation_failed", kind)
		return c.wrapError(err, "validation failed", obj)
	}

	// Step 2: Delete from K8s first
	err := c.k8s.DeleteFromK8s(ctx, obj, opts)
	if err != nil && !apiErrors.IsNotFound(err) {
		c.recordError(operation, "k8s_delete_failed", kind)
		return c.wrapError(err, "failed to delete from K8s", obj)
	}

	// Step 3: Delete from metadata storage if enabled
	if c.config.EnableMetadataStorage && c.metadata != nil {
		if metaErr := c.metadata.DeleteFromMetadata(ctx, obj); metaErr != nil {
			c.logger.Error(metaErr, "Failed to delete from metadata storage",
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
			// Don't fail the operation if metadata delete fails
		}
	}

	// Step 4: Delete from blob storage if needed
	if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
		if blobErr := c.blob.DeleteBlob(ctx, obj); blobErr != nil {
			c.logger.Error(blobErr, "Failed to delete blob storage",
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
			// Don't fail the operation if blob delete fails
		}
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.Info("Successfully completed delete operation",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"duration", time.Since(start),
	)

	return nil
}

// List implements api.Handler.List.
func (c *CompositeAPIHandler) List(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error {
	start := time.Now()
	operation := "List"
	kind := c.getListKind(list)

	c.logger.V(1).Info("Starting list operation",
		"namespace", namespace,
		"kind", kind,
	)

	// Prefer metadata storage for listing if enabled, otherwise use K8s
	if c.config.EnableMetadataStorage && c.metadata != nil {
		err := c.metadata.ListFromMetadata(ctx, namespace, opts, list)
		if err != nil {
			c.recordError(operation, "metadata_list_failed", kind)
			return c.wrapError(err, "failed to list from metadata storage", nil)
		}
	} else {
		err := c.k8s.ListFromK8s(ctx, namespace, opts, list)
		if err != nil {
			c.recordError(operation, "k8s_list_failed", kind)
			return c.wrapError(err, "failed to list from K8s", nil)
		}
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.V(1).Info("Successfully completed list operation",
		"namespace", namespace,
		"duration", time.Since(start),
	)

	return nil
}

// DeleteCollection implements api.Handler.DeleteCollection.
func (c *CompositeAPIHandler) DeleteCollection(ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error {
	start := time.Now()
	operation := "DeleteCollection"
	kind := c.getObjectKind(objType)

	c.logger.Info("Starting delete collection operation",
		"namespace", namespace,
		"kind", kind,
	)

	// If metadata storage is not enabled, use K8s directly
	if !c.config.EnableMetadataStorage || c.metadata == nil {
		err := c.k8s.DeleteCollectionFromK8s(ctx, objType, namespace, deleteOpts, listOpts)
		if err != nil {
			c.recordError(operation, "k8s_delete_collection_failed", kind)
			return c.wrapError(err, "failed to delete collection from K8s", objType)
		}

		c.recordSuccess(operation, time.Since(start), kind)
		return nil
	}

	// For metadata storage, we need to list and delete individually
	// Create a list object for the type
	gvk, err := ctrlRTApiutil.GVKForObject(objType, scheme.Scheme)
	if err != nil {
		c.recordError(operation, "gvk_resolution_failed", kind)
		return c.wrapError(err, "failed to resolve GVK", objType)
	}

	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	newObj, err := scheme.Scheme.New(listGVK)
	if err != nil {
		c.recordError(operation, "list_object_creation_failed", kind)
		return c.wrapError(err, "failed to create list object", objType)
	}

	listObj, ok := newObj.(ctrlRTClient.ObjectList)
	if !ok {
		c.recordError(operation, "list_interface_failed", kind)
		return status.Errorf(codes.Internal, "new object does not implement ObjectList interface")
	}

	// List objects
	err = c.metadata.ListFromMetadata(ctx, namespace, listOpts, listObj)
	if err != nil {
		c.recordError(operation, "list_failed", kind)
		return c.wrapError(err, "failed to list objects for deletion", objType)
	}

	// Extract items and delete individually
	items, err := meta.ExtractList(listObj)
	if err != nil || len(items) == 0 {
		c.recordSuccess(operation, time.Since(start), kind)
		return nil
	}

	// Delete items concurrently if enabled
	if c.config.ConcurrentOperations {
		// Convert runtime.Object slice to interface{} slice
		interfaceItems := make([]interface{}, len(items))
		for i, item := range items {
			interfaceItems[i] = item
		}
		return c.deleteItemsConcurrently(ctx, interfaceItems, deleteOpts, operation, start, kind)
	}

	// Delete items sequentially
	for _, item := range items {
		if obj, ok := item.(ctrlRTClient.Object); ok {
			if err := c.Delete(ctx, obj, deleteOpts); err != nil {
				c.recordError(operation, "item_delete_failed", kind)
				return c.wrapError(err, "failed to delete item", obj)
			}
		}
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.Info("Successfully completed delete collection operation",
		"namespace", namespace,
		"duration", time.Since(start),
		"items_deleted", len(items),
	)

	return nil
}

// Helper methods

func (c *CompositeAPIHandler) deleteItemsConcurrently(ctx context.Context, items []interface{}, deleteOpts *metav1.DeleteOptions, operation string, start time.Time, kind string) error {
	// Limit concurrency
	maxConcurrency := c.config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // default
	}

	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, len(items))

	for _, item := range items {
		if obj, ok := item.(ctrlRTClient.Object); ok {
			wg.Add(1)
			go func(obj ctrlRTClient.Object) {
				defer wg.Done()
				semaphore <- struct{}{} // acquire
				defer func() { <-semaphore }() // release

				if err := c.Delete(ctx, obj, deleteOpts); err != nil {
					errCh <- err
				}
			}(obj)
		}
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		c.recordError(operation, "concurrent_delete_failed", kind)
		return err
	}

	c.recordSuccess(operation, time.Since(start), kind)
	c.logger.Info("Successfully completed concurrent delete collection operation",
		"duration", time.Since(start),
		"items_deleted", len(items),
	)

	return nil
}

func (c *CompositeAPIHandler) getObjectKind(obj ctrlRTClient.Object) string {
	if obj == nil {
		return "unknown"
	}
	
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		if typeMeta, err := utils.GetObjectTypeMetafromObject(obj, scheme.Scheme); err == nil {
			kind = typeMeta.Kind
		}
	}
	if kind == "" {
		kind = "unknown"
	}
	return kind
}

func (c *CompositeAPIHandler) getListKind(list ctrlRTClient.ObjectList) string {
	if list == nil {
		return "unknown"
	}
	
	kind := list.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		kind = "unknown"
	}
	return kind
}

func (c *CompositeAPIHandler) wrapError(err error, message string, obj ctrlRTClient.Object) error {
	if obj != nil {
		return status.Errorf(codes.Internal, "%s: namespace=%s, name=%s, error=%v", 
			message, obj.GetNamespace(), obj.GetName(), err)
	}
	return status.Errorf(codes.Internal, "%s: error=%v", message, err)
}

func (c *CompositeAPIHandler) recordSuccess(operation string, duration time.Duration, kind string) {
	if c.metrics != nil {
		labels := map[string]string{
			"operation": operation,
			"kind":      kind,
			"status":    "success",
		}
		c.metrics.RecordAPILatency(operation, float64(duration.Milliseconds()), labels)
	}
}

func (c *CompositeAPIHandler) recordError(operation, errorCode, kind string) {
	if c.metrics != nil {
		labels := map[string]string{
			"kind": kind,
		}
		c.metrics.RecordAPIError(operation, errorCode, labels)
	}
}

func (c *CompositeAPIHandler) setUpdateTimestamp(obj ctrlRTClient.Object, hasSpecChange bool) {
	// Implementation would mirror the original setUpdateTimestamp function
	// This is a placeholder for the actual timestamp setting logic
	c.logger.V(2).Info("Setting update timestamp",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
		"hasSpecChange", hasSpecChange,
	)
}

func (c *CompositeAPIHandler) hasSpecChange(ctx context.Context, obj ctrlRTClient.Object) (bool, error) {
	// Implementation would mirror the original hasSpecChange function
	// This is a placeholder for the actual spec change detection logic
	c.logger.V(2).Info("Checking for spec changes",
		"namespace", obj.GetNamespace(),
		"name", obj.GetName(),
	)
	return true, nil // For now, assume there are always spec changes
}

func (c *CompositeAPIHandler) isDeletedImmutableObject(obj ctrlRTClient.Object) bool {
	return utils.IsImmutable(obj) && obj.GetDeletionTimestamp() != nil
}

func (c *CompositeAPIHandler) handleUpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error {
	if c.metadata == nil {
		return status.Errorf(codes.Internal, "metadata storage not configured")
	}

	err := c.metadata.UpdateInMetadata(ctx, obj)
	if err != nil {
		return err
	}

	// Handle blob storage if needed
	if c.config.EnableBlobStorage && c.blob != nil && c.blob.IsObjectInteresting(obj) {
		if blobErr := c.blob.StoreBlob(ctx, obj); blobErr != nil {
			c.logger.Error(blobErr, "Failed to update blob storage during metadata update",
				"namespace", obj.GetNamespace(),
				"name", obj.GetName(),
			)
			// Don't fail the operation if blob storage fails
		}
	}

	return nil
}