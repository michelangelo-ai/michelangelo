package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sHandler handles only Kubernetes operations - wraps existing k8sClient functionality
type K8sHandler interface {
	CreateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error
	GetFromK8s(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error
	UpdateInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error
	UpdateStatusInK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error
	DeleteFromK8s(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error
	ListFromK8s(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error
	DeleteCollectionFromK8s(ctx context.Context, objType ctrlRTClient.Object, namespace string, deleteOpts *metav1.DeleteOptions, listOpts *metav1.ListOptions) error
}

// MetadataHandler handles only metadata storage operations - wraps existing metadataStorage functionality
type MetadataHandler interface {
	GetFromMetadata(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error
	UpdateInMetadata(ctx context.Context, obj ctrlRTClient.Object) error
	DeleteFromMetadata(ctx context.Context, obj ctrlRTClient.Object) error
	ListFromMetadata(ctx context.Context, namespace string, opts *metav1.ListOptions, list ctrlRTClient.ObjectList) error
}

// BlobHandler handles only blob storage operations - wraps existing blobStorage functionality
type BlobHandler interface {
	IsObjectInteresting(obj ctrlRTClient.Object) bool
	MergeWithExternalBlob(ctx context.Context, obj ctrlRTClient.Object) error
	DeleteFromBlobStorage(ctx context.Context, obj ctrlRTClient.Object) error
}

// ValidationHandler handles only validation operations - wraps existing api.Validate functionality
type ValidationHandler interface {
	ValidateCreate(obj ctrlRTClient.Object) error
	ValidateUpdate(obj ctrlRTClient.Object) error
	ValidateDelete(obj ctrlRTClient.Object) error
}