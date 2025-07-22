package minio

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"
	"log"

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
	log.Printf(">>>>>>>>>>>newClient: initializing with config - UseEnvAws: %v, UseIAM: %v, AwsEndpointUrl: %s", config.UseEnvAws, config.UseIAM, config.AwsEndpointUrl)
	var creds *credentials.Credentials
	if config.UseEnvAws {
		log.Printf(">>>>>>>>>>>newClient: using environment AWS credentials")
		creds = credentials.NewEnvAWS()
	} else if config.UseIAM {
		log.Printf(">>>>>>>>>>>newClient: using IAM credentials with endpoint: http://169.254.169.254")
		creds = credentials.NewIAM("http://169.254.169.254")
	} else {
		log.Printf(">>>>>>>>>>>newClient: using static credentials")
		creds = credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, "")
	}

	log.Printf(">>>>>>>>>>>newClient: creating minio client with endpoint: %s, secure: false", config.AwsEndpointUrl)
	s3Client, err := minio.New(config.AwsEndpointUrl, &minio.Options{
		Creds:  creds,
		Secure: false,
	})
	if err != nil {
		log.Printf(">>>>>>>>>>>newClient: failed to create minio client: %v", err)
		return BlobStoreClientOut{}, err
	}
	log.Printf(">>>>>>>>>>>newClient: successfully created minio client")
	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{s3Client: s3Client, scheme: "s3"},
	}, nil
}
