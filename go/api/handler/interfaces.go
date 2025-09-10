package handler

import (
	"context"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandler abstracts Kubernetes API operations for improved testability and separation of concerns.
//
// This interface follows the adapter pattern commonly used in Kubernetes operators
// and maintains the same semantics as controller-runtime client.Client.
// All operations support standard Kubernetes features like optimistic concurrency,
// field/label selectors, and graceful deletion.
//
// Example usage:
//
//	handler := NewK8sHandler(client)
//	if err := handler.CreateInK8s(ctx, obj, &metav1.CreateOptions{}); err != nil {
//		return fmt.Errorf("failed to create object: %w", err)
//	}
type K8sHandler interface {
	// CreateInK8s creates a new object in the Kubernetes cluster.
	// Returns an error if the object already exists or creation fails.
	CreateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error

	// GetFromK8s retrieves an object from the Kubernetes cluster by namespace and name.
	// Returns NotFound error if the object doesn't exist.
	GetFromK8s(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error

	// UpdateInK8s updates an existing object in the Kubernetes cluster.
	// Uses optimistic concurrency control based on resourceVersion.
	UpdateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error

	// UpdateStatusInK8s updates only the status subresource of an object.
	// This is separated from spec updates to avoid conflicts in controllers.
	UpdateStatusInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error

	// DeleteFromK8s removes an object from the Kubernetes cluster.
	// Supports graceful deletion with configurable grace periods.
	DeleteFromK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error

	// ListFromK8s retrieves a list of objects matching the given criteria.
	// Supports field and label selectors, pagination, and namespace scoping.
	ListFromK8s(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error

	// DeleteCollectionFromK8s removes multiple objects matching the given criteria.
	// Combines listing and deletion operations with proper error handling.
	DeleteCollectionFromK8s(ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error
}

// MetadataHandler abstracts metadata storage operations for pluggable storage backends.
//
// This interface enables persisting object metadata beyond the Kubernetes API server
// lifecycle, supporting features like soft deletion and extended retention policies.
// The storage layer maintains consistency with Kubernetes API semantics.
//
// Example usage:
//
//	handler := NewMetadataHandler(storage, blobStorage, logger)
//	if err := handler.UpdateInMetadata(ctx, obj); err != nil {
//		return fmt.Errorf("failed to persist metadata: %w", err)
//	}
type MetadataHandler interface {
	// GetFromMetadata retrieves an object from the metadata storage by namespace and name.
	// This is typically used as a fallback when objects are not found in Kubernetes.
	GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error

	// UpdateInMetadata persists or updates an object in the metadata storage.
	// This operation is idempotent and handles both creation and updates.
	UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error

	// DeleteFromMetadata removes an object from the metadata storage.
	// This may also trigger cleanup of associated blob storage if configured.
	DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error

	// ListFromMetadata retrieves objects from metadata storage matching the given criteria.
	// Supports the same filtering options as the Kubernetes API, with optional extended options.
	ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, list ctrlRTClient.ObjectList) error
}

// BlobHandler abstracts blob storage operations for large object data.
//
// This interface supports storing large objects separately from metadata using
// backends like S3 or GCS, following patterns used by Kubernetes for objects
// that exceed etcd size limits or require specialized storage characteristics.
//
// Example usage:
//
//	handler := NewBlobHandler(storage)
//	if handler.IsObjectInteresting(obj) {
//		if err := handler.MergeWithExternalBlob(ctx, obj); err != nil {
//			return fmt.Errorf("failed to merge blob data: %w", err)
//		}
//	}
type BlobHandler interface {
	// IsObjectInteresting determines if an object should be stored in blob storage.
	// This typically checks object size, type, or annotations to make the decision.
	IsObjectInteresting(obj ctrlRTClient.Object) bool

	// MergeWithExternalBlob retrieves blob data and merges it with the object.
	// This is used during object retrieval to reconstitute the complete object.
	MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error

	// DeleteFromBlobStorage removes blob data associated with an object.
	// This is called during object deletion to prevent storage leaks.
	DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error
}

// ValidationHandler abstracts object validation operations for consistent validation logic.
//
// This interface centralizes validation across API operations, following patterns
// used by Kubernetes admission controllers. Validation ensures data integrity
// and business rule compliance before persisting objects.
//
// Example usage:
//
//	handler := NewValidationHandler()
//	if err := handler.ValidateCreate(obj); err != nil {
//		return fmt.Errorf("validation failed: %w", err)
//	}
type ValidationHandler interface {
	// ValidateCreate validates an object before creation.
	// This includes schema validation, business rules, and resource constraints.
	ValidateCreate(obj ctrlRTClient.Object) error

	// ValidateUpdate validates an object before updates.
	// This may include additional checks for immutable fields and state transitions.
	ValidateUpdate(obj ctrlRTClient.Object) error

	// ValidateDelete validates whether an object can be safely deleted.
	// This may check for dependencies, finalizers, or business constraints.
	ValidateDelete(obj ctrlRTClient.Object) error
}
