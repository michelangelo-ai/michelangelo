package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	mtags "github.com/minio/minio-go/v7/pkg/tags"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)


// NewMinioBlobStorageClient creates a new MinIO client.
func NewMinioBlobStorageClient(cfg Config) (*minio.Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}
	return client, nil
}

type MinioBlobStorage struct {
	client     *minio.Client
	bucketName string
}

// NewMinioBlobStorage creates a new MinioBlobStorage instance
func NewMinioBlobStorage(conf Config) (storage.BlobStorage, error) {
	client, err := minio.New(conf.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.AccessKey, conf.SecretKey, ""),
		Secure: conf.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	// Ensure bucket exists
	exists, err := client.BucketExists(context.Background(), conf.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = client.MakeBucket(context.Background(), conf.BucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return MinioBlobStorage{
		client:     client,
		bucketName:  conf.BucketName,
	}, nil
}

// UploadBlob uploads data to the given path and returns the link for download
func (m MinioBlobStorage) UploadBlob(ctx context.Context, path string, data []byte) (string, error) {
	r := bytes.NewReader(data)
	_, err := m.client.PutObject(ctx, m.bucketName, path, r, int64(len(data)), minio.PutObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to upload blob: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", m.client.EndpointURL().String(), m.bucketName, path), nil
}

// DownloadBlob downloads the data in the given path
func (m MinioBlobStorage) DownloadBlob(ctx context.Context, path string) ([]byte, error) {
	obj, err := m.client.GetObject(ctx, m.bucketName, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer obj.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read object data: %w", err)
	}

	return buf.Bytes(), nil
}

// DeleteBlob deletes the data in the given path
func (m *MinioBlobStorage) DeleteBlob(ctx context.Context, path string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, path, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// UpdateTags updates the tags for the blob.
func (m MinioBlobStorage) UpdateTags(ctx context.Context, obj runtime.Object, tags map[string]string) error {
	path := generateBlobPath(obj) // Replace this with your logic to derive the blob path

	convertedTags := &mtags.Tags{}
	for key, value := range tags {
		convertedTags.Set(key, value)
	}

	err := m.client.SetBucketTagging(ctx, m.bucketName, convertedTags)
	if err != nil {
		return fmt.Errorf("failed to update tags for object at path %s: %w", path, err)
	}

	return nil
}

// MergeWithExternalBlob gets the corresponding object stored in the blob storage and does the merge.
func (m MinioBlobStorage) MergeWithExternalBlob(ctx context.Context, obj runtime.Object) error {
	path := generateBlobPath(obj) // Replace this with your logic to derive the blob path
	externalData, err := m.DownloadBlob(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to download blob for merging: %w", err)
	}

	// Assuming the runtime.Object can be marshaled to and unmarshaled from JSON
	currentData, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	mergedData := mergeJSON(currentData, externalData) // Replace this with your merge logic
	err = json.Unmarshal(mergedData, obj)
	if err != nil {
		return fmt.Errorf("failed to unmarshal merged data: %w", err)
	}

	return nil
}

// UploadToBlobStorage uploads the object to blob storage and returns the key to retrieve the object.
func (m MinioBlobStorage) UploadToBlobStorage(ctx context.Context, obj runtime.Object) (string, error) {
	path := generateBlobPath(obj) // Replace this with your logic to derive the blob path
	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: %w", err)
	}

	link, err := m.UploadBlob(ctx, path, data)
	if err != nil {
		return "", fmt.Errorf("failed to upload object to blob storage: %w", err)
	}

	return link, nil
}

// DeleteFromBlobStorage deletes the object from blob storage.
func (m MinioBlobStorage) DeleteFromBlobStorage(ctx context.Context, obj runtime.Object) error {
	path := generateBlobPath(obj) // Replace this with your logic to derive the blob path
	err := m.DeleteBlob(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to delete object from blob storage: %w", err)
	}
	return nil
}

// IsObjectInteresting determines whether the object should be processed by blob storage.
func (m MinioBlobStorage) IsObjectInteresting(obj runtime.Object) bool {
	// Replace this with your logic to determine if the object is relevant for blob storage
	return true
}

// Helper functions
func generateBlobPath(obj runtime.Object) string {
	// Logic to generate a unique blob path based on the runtime.Object metadata
	// For example, using the object's namespace and name
	meta, ok := obj.(metav1.Object)
	if !ok {
		panic("object does not implement metav1.Object")
	}
	return fmt.Sprintf("%s/%s", meta.GetNamespace(), meta.GetName())
}

func mergeJSON(current, external []byte) []byte {
	// Replace this with your JSON merge logic (e.g., deep merge of two JSON objects)
	return current // Placeholder: Implement proper merging logic
}
