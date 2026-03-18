package blobstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// _blobKeyPattern is "s3://bucket/kind/namespace/name/uid"
const _blobKeyPattern = "s3://%s/%s/%s/%s/%s"

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
		fmt.Printf("[BLOBSTORAGE] IsObjectInteresting: no EnabledCRDs configured, returning false\n")
		return false
	}
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		t := reflect.TypeOf(obj)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		kind = t.Name()
		fmt.Printf("[BLOBSTORAGE] IsObjectInteresting: GVK kind empty, using reflect kind=%q\n", kind)
	}
	result := h.config.EnabledCRDs[strings.ToLower(kind)]
	fmt.Printf("[BLOBSTORAGE] IsObjectInteresting: kind=%q, enabledCRDs=%v, result=%v\n", kind, h.config.EnabledCRDs, result)
	return result
}

// UploadToBlobStorage JSON-marshals the full object and uploads it to blob storage.
// Uses a fixed key based on the object's UID so no annotation is needed.
func (h *handler) UploadToBlobStorage(ctx context.Context, obj runtime.Object) (string, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", fmt.Errorf("failed to get object accessor: %w", err)
	}

	fmt.Printf("[BLOBSTORAGE] UploadToBlobStorage: name=%q namespace=%q rv=%q\n",
		accessor.GetName(), accessor.GetNamespace(), accessor.GetResourceVersion())

	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: %w", err)
	}

	key := h.getKey(obj, accessor)
	fmt.Printf("[BLOBSTORAGE] UploadToBlobStorage: uploading to key=%q size=%d bytes\n", key, len(data))
	if err := h.store.Put(ctx, key, data); err != nil {
		fmt.Printf("[BLOBSTORAGE] UploadToBlobStorage: PUT failed: %v\n", err)
		return "", fmt.Errorf("failed to upload object to blob storage: %w", err)
	}
	fmt.Printf("[BLOBSTORAGE] UploadToBlobStorage: success key=%q\n", key)

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

	if string(accessor.GetUID()) == "" {
		return nil
	}

	key := h.getKey(obj, accessor)
	data, err := h.store.Get(ctx, key)
	if err != nil {
		// Blob may not exist for objects created before blob storage was enabled — treat as no-op.
		fmt.Printf("[BLOBSTORAGE] MergeWithExternalBlob: GET failed (blob may not exist) key=%q err=%v\n", key, err)
		return nil
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

// DeleteFromBlobStorage removes the object's blob using the UID-based key.
func (h *handler) DeleteFromBlobStorage(ctx context.Context, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Errorf("failed to get object accessor: %w", err)
	}

	if string(accessor.GetUID()) == "" {
		return nil
	}

	key := h.getKey(obj, accessor)
	if err := h.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete object from blob storage: %w", err)
	}

	return nil
}

// UpdateTags is a no-op for Phase 1. Minio object tagging will be added in Phase 2.
func (h *handler) UpdateTags(_ context.Context, _ runtime.Object, _ map[string]string) error {
	return nil
}

func (h *handler) getKey(obj runtime.Object, accessor metav1.Object) string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		t := reflect.TypeOf(obj)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		kind = t.Name()
	}
	key := fmt.Sprintf(_blobKeyPattern,
		h.config.BucketName,
		strings.ToLower(kind),
		accessor.GetNamespace(),
		accessor.GetName(),
		string(accessor.GetUID()),
	)
	fmt.Printf("[BLOBSTORAGE] getKey: kind=%q key=%q\n", kind, key)
	return key
}
