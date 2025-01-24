package ray

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
)

var Workflows = (*workflows)(nil)

type workflows struct{}

// TODO: andrii: implement Ray workflows once Ray API is ready

func (r *workflows) CreateRayCluster(ctx workflow.Context, request any) (any, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("workflow", zap.Any("request", request))

	var response any
	if err := workflow.ExecuteActivity(ctx, ray.Activities.CreateRayCluster, request).Get(ctx, &response); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}
	return response, nil
}
