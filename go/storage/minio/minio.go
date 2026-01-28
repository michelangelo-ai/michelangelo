package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"

	proto "github.com/gogo/protobuf/proto"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/tags"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// Config holds MinIO configuration
type Config struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"accessKeyID"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	UseSSL          bool   `yaml:"useSSL"`
	BucketName      string `yaml:"bucketName"`
}

// minioBlobStorage implements storage.BlobStorage using MinIO
type minioBlobStorage struct {
	client *minio.Client
	config Config
}

// NewBlobStorage creates a new MinIO blob storage
func NewBlobStorage(config Config) (storage.BlobStorage, error) {
	// Initialize MinIO client
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Check if bucket exists, create if not
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if bucket exists: %w", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, config.BucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &minioBlobStorage{
		client: client,
		config: config,
	}, nil
}

// MergeWithExternalBlob gets the corresponding object stored in blob storage and merges it
func (m *minioBlobStorage) MergeWithExternalBlob(ctx context.Context, object runtime.Object) error {
	if !m.IsObjectInteresting(object) {
		return nil
	}

	objectKey := m.getObjectKey(object)

	// Get object from MinIO
	obj, err := m.client.GetObject(ctx, m.config.BucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to get object from blob storage: %w", err)
	}
	defer obj.Close()

	// Read object data
	data, err := io.ReadAll(obj)
	if err != nil {
		return fmt.Errorf("failed to read object data: %w", err)
	}

	// Unmarshal into the object
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return fmt.Errorf("object does not implement proto.Message")
	}

	if err := proto.Unmarshal(data, protoMsg); err != nil {
		return fmt.Errorf("failed to unmarshal blob data: %w", err)
	}

	// Fill blob fields if object implements ObjectWithBlobFields
	if blobObj, ok := object.(storage.ObjectWithBlobFields); ok {
		blobObj.FillBlobFields(object)
	}

	return nil
}

// UploadToBlobStorage uploads the object to blob storage
func (m *minioBlobStorage) UploadToBlobStorage(ctx context.Context, object runtime.Object) (string, error) {
	if !m.IsObjectInteresting(object) {
		return "", nil
	}

	// Clear blob fields before uploading if object implements ObjectWithBlobFields
	if blobObj, ok := object.(storage.ObjectWithBlobFields); ok {
		blobObj.ClearBlobFields()
	}

	// Marshal object to protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return "", fmt.Errorf("object does not implement proto.Message")
	}

	data, err := proto.Marshal(protoMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: %w", err)
	}

	objectKey := m.getObjectKey(object)

	// Upload to MinIO
	reader := bytes.NewReader(data)
	_, err = m.client.PutObject(ctx, m.config.BucketName, objectKey, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/x-protobuf",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to blob storage: %w", err)
	}

	return objectKey, nil
}

// DeleteFromBlobStorage deletes the object from blob storage
func (m *minioBlobStorage) DeleteFromBlobStorage(ctx context.Context, object runtime.Object) error {
	if !m.IsObjectInteresting(object) {
		return nil
	}

	objectKey := m.getObjectKey(object)

	err := m.client.RemoveObject(ctx, m.config.BucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete from blob storage: %w", err)
	}

	return nil
}

// UpdateTags updates the tags for the blob
func (m *minioBlobStorage) UpdateTags(ctx context.Context, object runtime.Object, objectTags map[string]string) error {
	if !m.IsObjectInteresting(object) {
		return nil
	}

	objectKey := m.getObjectKey(object)

	// Convert tags to MinIO format
	minioTags, err := tags.NewTags(objectTags, false)
	if err != nil {
		return fmt.Errorf("failed to create tags: %w", err)
	}

	err = m.client.PutObjectTagging(ctx, m.config.BucketName, objectKey, minioTags, minio.PutObjectTaggingOptions{})
	if err != nil {
		return fmt.Errorf("failed to update tags: %w", err)
	}

	return nil
}

// IsObjectInteresting returns whether the object should be processed by blob storage
func (m *minioBlobStorage) IsObjectInteresting(object runtime.Object) bool {
	// Check if object implements ObjectWithBlobFields interface
	blobObj, ok := object.(storage.ObjectWithBlobFields)
	if !ok {
		return false
	}

	return blobObj.HasBlobFields()
}

// getObjectKey generates the storage key for an object
func (m *minioBlobStorage) getObjectKey(object runtime.Object) string {
	metaObj, err := meta.Accessor(object)
	if err != nil {
		return ""
	}

	gvk := object.GetObjectKind().GroupVersionKind()

	// Format: <group>/<version>/<kind>/<namespace>/<name>/<uid>
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		gvk.Group,
		gvk.Version,
		gvk.Kind,
		metaObj.GetNamespace(),
		metaObj.GetName(),
		metaObj.GetUID(),
	)
}
