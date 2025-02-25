package storage

import (
	"context"
	"fmt"
	"go.uber.org/cadence"
	"go.uber.org/cadence/activity"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
)

var Activities = (*activities)(nil)

type activities struct {
	impls map[string]Storage
}

// Implement the Read method for the S3Activities struct
func (a *activities) Read(ctx context.Context, protocol string, path string) (any, *cadence.CustomError) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("path", path))
	if impl, ok := a.impls[protocol]; ok {
		result, err := impl.Read(ctx, path)
		if err != nil {
			return nil, cadence.NewCustomError(yarpcerrors.FromError(err).Code().String(), err.Error())
		}
		return result, nil
	}
	return nil, cadence.NewCustomError(fmt.Sprintf("protocol %s is not supported"), protocol)
}
