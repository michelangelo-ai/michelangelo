package minio

import (
	"fmt"

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

// newClient initializes new S3/MinIO storage clients using the provided configuration.
// It creates clients for multiple S3/MinIO storage providers based on the configuration map.
// Returns multiple BlobStoreClientOut instances or an error if initialization fails.
func newClient(config Config) ([]BlobStoreClientOut, error) {
	var clients []BlobStoreClientOut

	// If no storage providers configured, create default AWS S3 client
	if len(config.StorageProviders) == 0 {
		defaultClient, err := newDefaultS3Client()
		if err != nil {
			return nil, fmt.Errorf("failed to create default S3 client: %w", err)
		}
		return []BlobStoreClientOut{defaultClient}, nil
	}

	// Create clients for each configured S3 storage provider
	for providerKey, providerConfig := range config.StorageProviders {
		if providerConfig.Type != "s3" {
			continue // Skip non-S3 providers
		}

		client, err := newS3ClientWithKey(providerKey, providerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create S3 client for provider %s: %w", providerKey, err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

// newDefaultS3Client creates a default S3 client when no providers are configured
func newDefaultS3Client() (BlobStoreClientOut, error) {
	// Use environment-based credentials for default client
	creds := credentials.NewEnvAWS()

	s3Client, err := minio.New("s3.amazonaws.com", &minio.Options{
		Creds:  creds,
		Secure: true,
	})
	if err != nil {
		return BlobStoreClientOut{}, err
	}

	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{
			s3Client:    s3Client,
			scheme:      "s3",
			providerKey: "aws-sandbox", // Default provider key
		},
	}, nil
}

// newS3ClientWithKey creates a new S3/MinIO client with provider key
func newS3ClientWithKey(providerKey string, config StorageProvider) (BlobStoreClientOut, error) {
	var creds *credentials.Credentials
	if config.UseEnvAws {
		creds = credentials.NewEnvAWS()
	} else if config.UseIAM {
		creds = credentials.NewIAM("")
	} else {
		creds = credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, "")
	}

	endpoint := config.AwsEndpointUrl
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  creds,
		Secure: true,
	})
	if err != nil {
		return BlobStoreClientOut{}, err
	}

	return BlobStoreClientOut{
		BlobStoreClient: &minioClient{
			s3Client:    s3Client,
			scheme:      "s3",
			providerKey: providerKey,
		},
	}, nil
}
