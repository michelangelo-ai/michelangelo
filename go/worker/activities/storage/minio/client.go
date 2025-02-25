package minio

import (
	"context"
	"io"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/minio/minio-go/v7"
)

type minioClient struct {
	s3Client *minio.Client
}

// Implement the Read method for the S3Activities struct
func (a *minioClient) Read(ctx context.Context, path string) (any, error) {
	parts := strings.SplitN(path, "/", 2)
	bucket := parts[0]
	filePath := parts[1]

	output, err := a.s3Client.GetObject(ctx, bucket, filePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(output)
	if err != nil {
		return nil, err
	}
	if err = output.Close(); err != nil {
		return nil, err
	}

	var res any
	err = jsoniter.Unmarshal(data, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (a *minioClient) Protocol() string {
	return "s3"
}
