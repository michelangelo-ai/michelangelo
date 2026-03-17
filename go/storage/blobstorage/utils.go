package blobstorage

import (
	"context"
	"reflect"

	"github.com/michelangelo-ai/michelangelo/go/api"
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
		// Fetch from MySQL first to obtain the BlobStorageUUIDAnnotation needed to locate the blob.
		objFromStorage := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
		getErr := metadataStorage.GetByID(ctx, string(obj.GetUID()), objFromStorage)

		if err := metadataStorage.Delete(ctx, typeMeta, obj.GetNamespace(), obj.GetName()); err != nil {
			return err
		}

		if getErr == nil {
			// Best-effort: failure to delete the blob is not critical.
			_ = blobStorage.DeleteFromBlobStorage(ctx, objFromStorage)
		}

		return nil
	}

	return metadataStorage.Delete(ctx, typeMeta, obj.GetNamespace(), obj.GetName())
}

// updateInternal handles the blob-storage upload path before upserting to MySQL.
// Steps:
//  1. Retrieve the current UUID annotation from MySQL (tracks last uploaded resource version).
//  2. Skip upload if resource version is unchanged.
//  3. Upload the full object to blob storage (sets BlobStorageUUIDAnnotation in memory).
//  4. Deep-copy the object, call ClearBlobFields on the copy, and upsert the stripped copy to
//     MySQL so that large payloads (step Input/Output, Conditions, etc.) are not stored in etcd.
//  5. Best-effort: mark the previous blob snapshot for cleanup via UpdateTags.
func updateInternal(
	ctx context.Context,
	obj client.Object,
	metadataStorage storage.MetadataStorage,
	direct bool,
	indexedFields []storage.IndexedField,
	blobStorage storage.BlobStorage,
) error {
	prevUUID := getUUIDFromMetadataStorage(ctx, obj, metadataStorage)

	// Nothing changed since the last upload — skip.
	if prevUUID != "" && obj.GetResourceVersion() == prevUUID {
		return nil
	}

	if _, err := blobStorage.UploadToBlobStorage(ctx, obj); err != nil {
		return err
	}

	// Phase 2: clear blob fields on a deep copy so that large payloads (step
	// Input/Output, Conditions, etc.) are not persisted in MySQL/etcd.
	// The original obj is left intact so the caller continues to see all fields.
	objToWrite := obj
	if blobFieldObj, ok := obj.(storage.ObjectWithBlobFields); ok && blobFieldObj.HasBlobFields() {
		copied := obj.DeepCopyObject().(client.Object)
		copied.(storage.ObjectWithBlobFields).ClearBlobFields()
		objToWrite = copied
	}

	if err := metadataStorage.Upsert(ctx, objToWrite, direct, indexedFields); err != nil {
		return err
	}

	// Best-effort: tag the old blob snapshot for eventual cleanup.
	if prevUUID != "" && prevUUID != obj.GetResourceVersion() {
		_ = blobStorage.UpdateTags(ctx, obj, map[string]string{"indirect-delete": "true"})
	}

	return nil
}

// getUUIDFromMetadataStorage fetches the BlobStorageUUIDAnnotation stored in MySQL for obj.
// Returns an empty string if the object is not found or has no annotation.
func getUUIDFromMetadataStorage(ctx context.Context, obj client.Object, metadataStorage storage.MetadataStorage) string {
	objFromStorage := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
	if err := metadataStorage.GetByID(ctx, string(obj.GetUID()), objFromStorage); err != nil {
		return ""
	}
	if annotations := objFromStorage.GetAnnotations(); annotations != nil {
		return annotations[api.BlobStorageUUIDAnnotation]
	}
	return ""
}
