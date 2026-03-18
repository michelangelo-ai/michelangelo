package blobstorage

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleUpdate syncs obj to metadataStorage and, if blob storage is configured and the object is
// interesting, also uploads the full object to blob storage.
// When blobStorage is nil it falls back to a direct MetadataStorage.Upsert call.
func HandleUpdate(
	ctx context.Context,
	obj client.Object,
	metadataStorage storage.MetadataStorage,
	direct bool,
	indexedFields []storage.IndexedField,
	blobStorage storage.BlobStorage,
) error {
	if !direct && blobStorage != nil && blobStorage.IsObjectInteresting(obj) {
		return updateInternal(ctx, obj, metadataStorage, direct, indexedFields, blobStorage)
	}
	return metadataStorage.Upsert(ctx, obj, direct, indexedFields)
}

// HandleDelete deletes obj from metadataStorage and, if blob storage is configured and the object
// is interesting, also removes the blob. Blob deletion is best-effort and does not fail the call.
// When blobStorage is nil it falls back to a direct MetadataStorage.Delete call.
func HandleDelete(
	ctx context.Context,
	typeMeta *metav1.TypeMeta,
	obj client.Object,
	metadataStorage storage.MetadataStorage,
	blobStorage storage.BlobStorage,
) error {
	if blobStorage != nil && blobStorage.IsObjectInteresting(obj) {
		if err := metadataStorage.Delete(ctx, typeMeta, obj.GetNamespace(), obj.GetName()); err != nil {
			return err
		}

		// Best-effort: failure to delete the blob is not critical.
		_ = blobStorage.DeleteFromBlobStorage(ctx, obj)

		return nil
	}

	return metadataStorage.Delete(ctx, typeMeta, obj.GetNamespace(), obj.GetName())
}

// updateInternal handles the blob-storage upload path before upserting to MySQL.
// Steps:
//  1. Upload the full object to blob storage using a fixed key (bucket/kind/ns/name/uid).
//  2. Deep-copy the object, call ClearBlobFields on the copy, and upsert the stripped copy to
//     MySQL so that large payloads (step Input/Output, Conditions, etc.) are not stored there.
//  3. Best-effort: tag previous blobs for cleanup via UpdateTags.
func updateInternal(
	ctx context.Context,
	obj client.Object,
	metadataStorage storage.MetadataStorage,
	direct bool,
	indexedFields []storage.IndexedField,
	blobStorage storage.BlobStorage,
) error {
	if _, err := blobStorage.UploadToBlobStorage(ctx, obj); err != nil {
		return err
	}

	// Clear blob fields on a deep copy so that large payloads (step
	// Input/Output, Conditions, etc.) are not persisted in MySQL.
	// The original obj is left intact so the caller continues to see all fields.
	objToWrite := obj
	if blobFieldObj, ok := obj.(storage.ObjectWithBlobFields); ok && blobFieldObj.HasBlobFields() {
		copied := obj.DeepCopyObject().(client.Object)
		copied.(storage.ObjectWithBlobFields).ClearBlobFields()
		objToWrite = copied
	}

	return metadataStorage.Upsert(ctx, objToWrite, direct, indexedFields)
}
