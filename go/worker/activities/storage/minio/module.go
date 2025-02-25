package minio

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"
)

type StorageOut struct {
	fx.Out
	Storage intf.Storage `group:"storages"`
}

// Module sets up dependency injection for the MinIO client.
// It calls newConfig to initialize configuration and newClient to create the MinIO client.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(newClient),
)

// newClient initializes a new minioClient using the provided configuration.
// It creates an underlying S3 client with static credentials.
// Returns a pointer to minioClient or an error if initialization fails.
func newClient(config Config) (StorageOut, error) {
	s3Client, err := minio.New(config.AwsEndpointUrl, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return StorageOut{}, err
	}
	return StorageOut{
		Storage: &minioClient{s3Client: s3Client},
	}, nil
}
