package minio

import (
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
	// Split the path into bucket and file path.
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	if parsedURL.Scheme != a.scheme {
		return nil, fmt.Errorf("scheme %s is not supported by minio client", parsedURL.Scheme)
	}
	bucket := parsedURL.Host
	filePath := parsedURL.Path[1:]

	// Retrieve the object from the specified bucket and file path.
	output, err := a.s3Client.GetObject(ctx, bucket, filePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Read all data from the retrieved object.
	data, err := io.ReadAll(output)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	// Close the object to release any associated resources.
	if err = output.Close(); err != nil {
		return nil, fmt.Errorf("failed to close object: %w", err)
	}

	return data, nil
}

// Scheme returns the scheme identifier used by this client.
func (a *minioClient) Scheme() string {
	return a.scheme
}
