package s3

import (
	"context"
	"fmt"
	"github.com/cadence-workflow/starlark-worker/ext"
	jsoniter "github.com/json-iterator/go"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/cadence"
	"go.uber.org/cadence/activity"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	"io"
)

var Activities = (*activities)(nil)

type activities struct {
	client *minio.Client
	config *Config
}

type ReadRequest struct {
	Bucket string `json:"bucket,omitempty"` // optional: client ID. Use default client if not set
	Path   string `json:"path,omitempty"`
}

// Implement the Read method for the S3Activities struct
func (a *activities) Read(ctx context.Context, req ReadRequest) (any, *cadence.CustomError) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", req.Path), zap.Any("bucket", req.Bucket))

	s3Client, err := minio.New(a.config.AwsEndpointUrl, &minio.Options{
		Creds:  credentials.NewStaticV4(a.config.AwsAccessKeyId, a.config.AwsSecretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, cadence.NewCustomError(yarpcerrors.FromError(err).Code().String(), err.Error())
	}

	output, err := s3Client.GetObject(ctx, req.Bucket, req.Path, minio.GetObjectOptions{})
	if err != nil {
		return nil, cadence.NewCustomError(yarpcerrors.FromError(err).Code().String(), err.Error())
	}

	data, err := io.ReadAll(output)
	if err != nil {
		return nil, cadence.NewCustomError(yarpcerrors.FromError(err).Code().String(), err.Error())
	}
	if err = output.Close(); err != nil {
		logger.Error("activity-error", ext.ZapError(err)...)
	}

	println("==================got json data============")
	fmt.Printf("+%s\n", data)
	var res any
	err = jsoniter.Unmarshal(data, &res)
	if err != nil {
		return nil, cadence.NewCustomError(yarpcerrors.FromError(err).Code().String(), err.Error())
	}
	fmt.Printf("+%v\n", res)
	return res, nil
}
