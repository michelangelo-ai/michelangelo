package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/minio/minio-go/v7"
)

var _ blobstore.BlobStoreClient = (*minioClient)(nil)

// minioClient is a wrapper around the MinIO S3 client.
// It provides methods to interact with S3-compatible storage.
type minioClient struct {
	s3Client *minio.Client
	scheme   string
}

// Get retrieves an object from S3 storage, reads its content,
// unmarshals the JSON data, and returns the result.
// The blobURI is expected to be in the format "s3://bucket/path".
func (a *minioClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	bucket, filePath, err := parseURI(blobURI, a.scheme)
	if err != nil {
		return nil, err
	}

	output, err := a.s3Client.GetObject(ctx, bucket, filePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	data, err := io.ReadAll(output)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	if err = output.Close(); err != nil {
		return nil, fmt.Errorf("failed to close object: %w", err)
	}

	return data, nil
}

// Put uploads data to S3 storage at the given URI.
// The blobURI is expected to be in the format "s3://bucket/path".
func (a *minioClient) Put(ctx context.Context, blobURI string, data []byte) error {
	bucket, filePath, err := parseURI(blobURI, a.scheme)
	if err != nil {
		return err
	}

	_, err = a.s3Client.PutObject(ctx, bucket, filePath, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}

	return nil
}

// Delete removes an object from S3 storage at the given URI.
// The blobURI is expected to be in the format "s3://bucket/path".
func (a *minioClient) Delete(ctx context.Context, blobURI string) error {
	bucket, filePath, err := parseURI(blobURI, a.scheme)
	if err != nil {
		return err
	}

	if err := a.s3Client.RemoveObject(ctx, bucket, filePath, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Scheme returns the scheme identifier used by this client.
func (a *minioClient) Scheme() string {
	return a.scheme
}

func parseURI(blobURI, scheme string) (bucket, filePath string, err error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse url: %w", err)
	}
	if parsedURL.Scheme != scheme {
		return "", "", fmt.Errorf("scheme %s is not supported by minio client", parsedURL.Scheme)
	}
	return parsedURL.Host, parsedURL.Path[1:], nil
}
