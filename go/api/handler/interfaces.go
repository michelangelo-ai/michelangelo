package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandler provides an interface for Kubernetes API operations.
// This interface abstracts the controller-runtime client for improved testability
// and separation of concerns, following the adapter pattern commonly used in
// Kubernetes operators like controller-runtime itself.
//
// All methods delegate to the underlying Kubernetes client and maintain
// the same semantics as the controller-runtime client.Client interface.
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

// MetadataHandler provides an interface for metadata storage operations.
// This interface abstracts the metadata storage layer to enable pluggable
// storage backends while maintaining consistent error handling and semantics.
//
// The metadata storage is used for persisting object metadata beyond the
// Kubernetes API server lifecycle, enabling features like soft deletion
// and extended retention policies.
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
	// Supports the same filtering options as the Kubernetes API.
	ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error
}

// BlobHandler provides an interface for blob storage operations.
// This interface abstracts blob storage backends (like S3, GCS) for storing
// large object data separately from metadata, following the pattern used by
// systems like Kubernetes itself for storing large objects.
//
// The blob storage is used for objects that exceed size limits or require
// specialized storage characteristics.
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

// ValidationHandler provides an interface for object validation operations.
// This interface centralizes validation logic and enables consistent
// validation across different API operations, following the pattern used
// by Kubernetes admission controllers.
//
// Validation is performed before persisting objects to ensure data
// integrity and business rule compliance.
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
