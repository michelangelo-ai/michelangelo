package minio

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

type BlobStoreClientOut struct {
	fx.Out
	BlobStoreClient blobstore.BlobStoreClient `group:"blobstore_clients"`
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
func newClient(config Config) (BlobStoreClientOut, error) {
	var creds *credentials.Credentials
	if config.UseEnvAws {
		creds = credentials.NewEnvAWS()
	} else if config.UseIAM {
		creds = credentials.NewIAM(config.AwsEndpointUrl)
	} else {
		creds = credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, "")
	}

	s3Client, err := minio.New(config.AwsEndpointUrl, &minio.Options{
		Creds:  creds,
		Secure: false,
	})
	if err != nil {
		return BlobStoreClientOut{}, err
	}
	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{s3Client: s3Client, scheme: "s3"},
	}, nil
}
