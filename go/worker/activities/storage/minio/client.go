package minio

import (
	"context"
	"io"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/minio/minio-go/v7"
)

// minioClient is a wrapper around the MinIO S3 client.
// It provides methods to interact with S3-compatible storage.
type minioClient struct {
	s3Client *minio.Client
}

// Read retrieves an object from S3 storage, reads its content,
// unmarshals the JSON data, and returns the result.
// It expects the path format "bucket/filePath".
func (a *minioClient) Read(ctx context.Context, path string) (any, error) {
	// Split the path into bucket and file path.
	parts := strings.SplitN(path, "/", 2)
	bucket := parts[0]
	filePath := parts[1]

	// Retrieve the object from the specified bucket and file path.
	output, err := a.s3Client.GetObject(ctx, bucket, filePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	// Read all data from the retrieved object.
	data, err := io.ReadAll(output)
	if err != nil {
		return nil, err
	}
	// Close the object to release any associated resources.
	if err = output.Close(); err != nil {
		return nil, err
	}

	// Unmarshal the JSON data into a generic interface.
	var res any
	err = jsoniter.Unmarshal(data, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Protocol returns the protocol identifier used by this client.
func (a *minioClient) Protocol() string {
	return "s3"
}
