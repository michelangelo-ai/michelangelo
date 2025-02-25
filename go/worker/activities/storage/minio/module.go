package minio

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(newConfig),
	fx.Provide(newClient),
)

func newClient(config Config) (*minioClient, error) {
	s3Client, err := minio.New(config.AwsEndpointUrl, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AwsAccessKeyId, config.AwsSecretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}
	return &minioClient{s3Client: s3Client}, nil
}
