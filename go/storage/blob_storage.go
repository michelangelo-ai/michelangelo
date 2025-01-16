//go:generate mamockgen BlobStorage
package storage

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
)

// BlobStorage defines the interface of a Blob storage.
type BlobStorage interface {
	// MergeWithExternalBlob gets the corresponding object stored in the blob storage and do the merge.
	MergeWithExternalBlob(context.Context, runtime.Object) error
	// UploadToBlobStorage uploads the object to blob storage.
	// Returns the key to retrieve the object if the upload succeeds.
	UploadToBlobStorage(context.Context, runtime.Object) (string, error)
	// DeleteFromBlobStorage deletes the object from blob storage.
	DeleteFromBlobStorage(context.Context, runtime.Object) error
	// UpdateTags updates the tags for the blob.
	UpdateTags(context.Context, runtime.Object, map[string]string) error
	// IsObjectInteresting returns whether the object should be processed by blob storage
	IsObjectInteresting(runtime.Object) bool
}
