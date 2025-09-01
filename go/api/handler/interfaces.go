package handler

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandler handles only Kubernetes operations.
// Inspired by Kubernetes focused controllers and Kubeflow's WorkflowClient interface.
type K8sHandler interface {
	// CreateInK8s creates an object in Kubernetes cluster only
	CreateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error
	
	// GetFromK8s retrieves an object from Kubernetes cluster only
	GetFromK8s(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error
	
	// UpdateInK8s updates an object in Kubernetes cluster only
	UpdateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error
	
	// UpdateStatusInK8s updates only the status of an object in Kubernetes
	UpdateStatusInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error
	
	// DeleteFromK8s deletes an object from Kubernetes cluster only
	DeleteFromK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error
	
	// ListFromK8s lists objects from Kubernetes cluster only
	ListFromK8s(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error
	
	// DeleteCollectionFromK8s deletes a collection of objects from Kubernetes only
	DeleteCollectionFromK8s(ctx context.Context, objType ctrlRTClient.Object, namespace string, 
		deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error
}

// MetadataHandler handles only metadata storage operations.
// Inspired by Flyte's Repository pattern and Kubeflow's store interfaces.
type MetadataHandler interface {
	// CreateInMetadata creates an object in metadata storage only
	CreateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error
	
	// GetFromMetadata retrieves an object from metadata storage only
	GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error
	
	// UpdateInMetadata updates an object in metadata storage only
	UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error
	
	// DeleteFromMetadata deletes an object from metadata storage only
	DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error
	
	// ListFromMetadata lists objects from metadata storage only
	ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error
	
	// CheckExistsInMetadata checks if an object exists in metadata storage
	CheckExistsInMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) (bool, error)
}

// BlobHandler handles only blob storage operations.
// Inspired by Flyte's storage abstraction pattern.
type BlobHandler interface {
	// IsObjectInteresting checks if the object needs blob storage handling
	IsObjectInteresting(obj ctrlRTClient.Object) bool
	
	// MergeWithBlob merges object with external blob data
	MergeWithBlob(ctx context.Context, obj ctrlRTClient.Object) error
	
	// StoreBlob stores large object data in blob storage
	StoreBlob(ctx context.Context, obj ctrlRTClient.Object) error
	
	// DeleteBlob removes blob data associated with the object
	DeleteBlob(ctx context.Context, obj ctrlRTClient.Object) error
}

// ValidationHandler handles only validation operations.
// Inspired by Flyte's validation pattern separation.
type ValidationHandler interface {
	// ValidateCreate validates an object for creation
	ValidateCreate(obj ctrlRTClient.Object) error
	
	// ValidateUpdate validates an object for update
	ValidateUpdate(obj ctrlRTClient.Object) error
	
	// ValidateDelete validates an object for deletion
	ValidateDelete(obj ctrlRTClient.Object) error
}

// MetricsHandler handles only metrics and observability.
// Inspired by Flyte's metrics separation pattern.
type MetricsHandler interface {
	// RecordAPILatency records the latency of an API operation
	RecordAPILatency(operation string, duration float64, labels map[string]string)
	
	// RecordAPIError records an API error
	RecordAPIError(operation string, errorCode string, labels map[string]string)
	
	// RecordStorageOperation records storage operation metrics
	RecordStorageOperation(storageType string, operation string, duration float64)
}


// Dependencies encapsulates all external dependencies.
// Inspired by Kubeflow's dependency injection pattern.
type Dependencies struct {
	// K8sClient for Kubernetes operations
	K8sClient ctrlRTClient.Client
	
	// MetadataStorage for metadata operations
	MetadataStorage storage.MetadataStorage
	
	// BlobStorage for blob operations
	BlobStorage storage.BlobStorage
	
	// Logger for structured logging
	Logger logr.Logger
	
	// Metrics for observability
	Metrics tally.Scope
}