package blobstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// _blobKeyPattern is "s3://bucket/kind/namespace/name/uid/resourceVersion"
const _blobKeyPattern = "s3://%s/%s/%s/%s/%s/%s"

// Config holds blob storage handler configuration.
type Config struct {
	// BucketName is the S3/minio bucket to store objects in.
	BucketName string `yaml:"bucketName"`
	// EnabledCRDs is a map of lowercase kind names to enabled flag.
	// If empty, blob storage is disabled for all kinds.
	EnabledCRDs map[string]bool `yaml:"enabledCRDs"`
}

type handler struct {
	store  *blobstore.BlobStore
	config Config
}

// New returns an implementation of storage.BlobStorage backed by a BlobStore.
func New(store *blobstore.BlobStore, config Config) storage.BlobStorage {
	return &handler{store: store, config: config}
}

// IsObjectInteresting returns true if the object's kind is enabled for blob storage.
func (h *handler) IsObjectInteresting(obj runtime.Object) bool {
	if len(h.config.EnabledCRDs) == 0 {
		return false
	}
	gvk := obj.GetObjectKind().GroupVersionKind()
	return h.config.EnabledCRDs[strings.ToLower(gvk.Kind)]
}

// UploadToBlobStorage JSON-marshals the full object and uploads it to blob storage.
// On success it sets BlobStorageUUIDAnnotation on the in-memory object to the current resource version.
func (h *handler) UploadToBlobStorage(ctx context.Context, obj runtime.Object) (string, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", fmt.Errorf("failed to get object accessor: %w", err)
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: %w", err)
	}

	key := h.getKey(obj, accessor, accessor.GetResourceVersion())
	if err := h.store.Put(ctx, key, data); err != nil {
		return "", fmt.Errorf("failed to upload object to blob storage: %w", err)
	}

	// Record the resource version so we can detect unchanged objects on the next reconcile.
	annotations := accessor.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[api.BlobStorageUUIDAnnotation] = accessor.GetResourceVersion()
	accessor.SetAnnotations(annotations)

	return key, nil
}

// MergeWithExternalBlob downloads the object from blob storage and merges it into obj.
// For objects that implement ObjectWithBlobFields, only the blob-specific fields (Spec/Status)
// are filled via FillBlobFields, preserving the ETCD/MySQL metadata on obj.
// For plain objects (full object stored in blob), a full JSON unmarshal is performed.
func (h *handler) MergeWithExternalBlob(ctx context.Context, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}

	annotations := accessor.GetAnnotations()
	uuid, ok := annotations[api.BlobStorageUUIDAnnotation]
	if !ok || uuid == "" {
		return nil
	}

	key := h.getKey(obj, accessor, uuid)
	data, err := h.store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to download object from blob storage: %w", err)
	}

	// If the object has blob fields, unmarshal into a new instance and fill only those
	// fields to preserve the current metadata (annotations, resourceVersion, etc.).
	if blobFieldObj, ok := obj.(storage.ObjectWithBlobFields); ok && blobFieldObj.HasBlobFields() {
		newObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(runtime.Object)
		if err := json.Unmarshal(data, newObj); err != nil {
			return fmt.Errorf("failed to unmarshal blob object from blob storage: %w", err)
		}
		blobFieldObj.FillBlobFields(newObj)
		return nil
	}

	if err := json.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("failed to unmarshal object from blob storage: %w", err)
	}

	return nil
}

// DeleteFromBlobStorage removes the object's blob using the UUID annotation as the key.
func (h *handler) DeleteFromBlobStorage(ctx context.Context, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}

	annotations := accessor.GetAnnotations()
	uuid, ok := annotations[api.BlobStorageUUIDAnnotation]
	if !ok || uuid == "" {
		return nil
	}

	key := h.getKey(obj, accessor, uuid)
	if err := h.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete object from blob storage: %w", err)
	}

	return nil
}

// UpdateTags is a no-op for Phase 1. Minio object tagging will be added in Phase 2.
func (h *handler) UpdateTags(_ context.Context, _ runtime.Object, _ map[string]string) error {
	return nil
}

func (h *handler) getKey(obj runtime.Object, accessor metav1.Object, uuid string) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf(_blobKeyPattern,
		h.config.BucketName,
		strings.ToLower(gvk.Kind),
		accessor.GetNamespace(),
		accessor.GetName(),
		string(accessor.GetUID()),
		uuid,
	)
}
