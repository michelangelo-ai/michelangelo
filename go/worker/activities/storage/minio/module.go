package minio

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"
)

// Module sets up dependency injection for the MinIO client.
// It calls newConfig to initialize configuration and newClient to create the MinIO client.
var Module = fx.Options(
	fx.Invoke(newConfig),
	fx.Provide(newClient),
)

// newClient initializes a new minioClient using the provided configuration.
// It creates an underlying S3 client with static credentials.
// Returns a pointer to minioClient or an error if initialization fails.
func newClient(config Config) (*minioClient, error) {
	// Initialize the MinIO S3 client with the given endpoint and credentials.
	s3Client, err := minio.New(config.AwsEndpointUrl, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, ""),
		Secure: false, // Set to false to use an insecure connection (HTTP).
	})
	if err != nil {
		return nil, err
	}
	return &minioClient{s3Client: s3Client}, nil
}
