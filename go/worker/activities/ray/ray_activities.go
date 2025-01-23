package ray

import (
	"context"
	"go.uber.org/cadence/activity"
	"go.uber.org/zap"
)

var Activities = (*activities)(nil)

// TODO: andrii: implement Ray activities once Ray API is ready

type activities struct{}

func (r *activities) CreateRayCluster(ctx context.Context, request any) (any, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity", zap.Any("request", request))
	return map[string]any{"stub": true}, nil
}

func (r *activities) GetRayCluster(ctx context.Context, request any) (any, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity", zap.Any("request", request))
	return map[string]any{"stub": true}, nil
}
