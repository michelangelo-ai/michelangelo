package minio

import (
	"context"
	"fmt"
	"io"
	"log"
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
	log.Printf(">>>>>>>>>>>GET: starting request for blobURI: %s", blobURI)
	// Split the path into bucket and file path.
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		log.Printf(">>>>>>>>>>>GET: failed to parse URL %s: %v", blobURI, err)
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	if parsedURL.Scheme != a.scheme {
		log.Printf(">>>>>>>>>>>GET: unsupported scheme %s, expected %s", parsedURL.Scheme, a.scheme)
		return nil, fmt.Errorf("scheme %s is not supported by minio client", parsedURL.Scheme)
	}
	bucket := parsedURL.Host
	filePath := parsedURL.Path[1:]
	log.Printf(">>>>>>>>>>>GET: parsed URL - bucket: %s, filePath: %s", bucket, filePath)

	// Retrieve the object from the specified bucket and file path.
	log.Printf(">>>>>>>>>>>GET: calling s3Client.GetObject for bucket: %s, filePath: %s", bucket, filePath)
	output, err := a.s3Client.GetObject(ctx, bucket, filePath, minio.GetObjectOptions{})
	if err != nil {
		log.Printf(">>>>>>>>>>>GET: failed to get object from S3: %v", err)
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	log.Printf(">>>>>>>>>>>GET: successfully retrieved object, reading data...")
	// Read all data from the retrieved object.
	data, err := io.ReadAll(output)
	if err != nil {
		log.Printf(">>>>>>>>>>>GET: failed to read object data: %v", err)
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	log.Printf(">>>>>>>>>>>GET: successfully read %d bytes of data", len(data))
	// Close the object to release any associated resources.
	if err = output.Close(); err != nil {
		log.Printf(">>>>>>>>>>>GET: failed to close object: %v", err)
		return nil, fmt.Errorf("failed to close object: %w", err)
	}

	log.Printf(">>>>>>>>>>>GET: successfully completed request for blobURI: %s", blobURI)
	return data, nil
}

// Scheme returns the scheme identifier used by this client.
func (a *minioClient) Scheme() string {
	return a.scheme
}
