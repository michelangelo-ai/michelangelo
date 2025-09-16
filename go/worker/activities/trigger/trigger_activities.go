package trigger

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/workflow"
	triggerrunUtil "github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

var Activities = (*activities)(nil)

// activities struct encapsulates the YARPC clients for pipeline run services.
type activities struct {
	pipelineRunService v2pb.PipelineRunServiceYARPCClient
}

// Parameter generator factory function
func getParameterGenerator(triggerType string) parameter.ParameterGenerator {
	switch triggerType {
	case triggerrunUtil.TriggerTypeCron, triggerrunUtil.TriggerTypeInterval:
		return &parameter.CronParameterGenerator{}
	// TODO: Add other parameter generators here, such as backfill and batch rerun
	default:
		return &parameter.CronParameterGenerator{}
	}
}

// CreatePipelineRun creates a new pipeline run using the provided request parameters.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the pipeline run to create.
//
// Returns:
// - *v2pb.PipelineRun: The created pipeline run.
// - error: Error information if the operation fails.
func (r *activities) CreatePipelineRun(ctx context.Context, request *v2pb.CreatePipelineRunRequest) (*v2pb.PipelineRun, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.Any("request", request))

	response, err := r.pipelineRunService.CreatePipelineRun(ctx, request)
	if err != nil || response == nil || response.PipelineRun == nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "CreatePipelineRun", err.Error())
	}

	logger.Info("activity-success", zap.String("pipeline_run_name", response.PipelineRun.Name))
	return response.PipelineRun, nil
}

// GenerateBatchRunParams generates parameters for batch execution.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - triggerRun: The trigger run containing batch policy configuration.
//
// Returns:
// - []Object: Array of parameter objects for batch execution.
// - error: Error information if the operation fails.
func (r *activities) GenerateBatchRunParams(ctx context.Context, triggerRun *v2pb.TriggerRun) ([][]parameter.Params, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.String("trigger_run", triggerRun.Name))

	// Get trigger type and appropriate parameter generator
	triggerType := triggerrunUtil.GetTriggerType(triggerRun)
	generator := getParameterGenerator(triggerType)

	// Use interface method to generate parameters
	batches, err := generator.GenerateBatchParams(triggerRun)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GenerateBatchParams", err.Error())
	}

	logger.Info("activity-success", zap.Int("batch_count", len(batches)), zap.String("trigger_type", triggerType))
	return batches, nil
}

// GenerateConcurrentRunParams generates parameters for concurrent execution.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - triggerRun: The trigger run containing parameter configuration.
//
// Returns:
// - []Object: Array of parameter objects for concurrent execution.
// - error: Error information if the operation fails.
func (r *activities) GenerateConcurrentRunParams(ctx context.Context, triggerRun *v2pb.TriggerRun) ([]parameter.Params, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.String("trigger_run", triggerRun.Name))

	// Get trigger type and appropriate parameter generator
	triggerType := triggerrunUtil.GetTriggerType(triggerRun)
	generator := getParameterGenerator(triggerType)

	// Use interface method to generate parameters
	params, err := generator.GenerateConcurrentParams(triggerRun)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GenerateConcurrentParams", err.Error())
	}

	logger.Info("activity-success", zap.Int("param_count", len(params)), zap.String("trigger_type", triggerType))
	return params, nil
}

// PipelineRunSensor monitors pipeline run status.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - pipelineRun: The pipeline run to monitor.
//
// Returns:
// - *v2pb.PipelineRun: The updated pipeline run status.
// - error: Error information if the operation fails.
func (r *activities) PipelineRunSensor(ctx context.Context, pipelineRun *v2pb.PipelineRun) (*v2pb.PipelineRun, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity-start", zap.String("pipeline_run", pipelineRun.Name), zap.String("namespace", pipelineRun.Namespace))

	if pipelineRun == nil {
		err := fmt.Errorf("pipeline run is nil")
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "InvalidInput", err.Error())
	}

	getRequest := &v2pb.GetPipelineRunRequest{
		Namespace: pipelineRun.Namespace,
		Name:      pipelineRun.Name,
	}

	response, err := r.pipelineRunService.GetPipelineRun(ctx, getRequest)
	if err != nil {
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GetPipelineRun", err.Error())
	}

	if response == nil || response.PipelineRun == nil {
		err := fmt.Errorf("empty response from pipeline run service")
		logger.Error("activity-error", zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "EmptyResponse", err.Error())
	}

	logger.Info("activity-success", zap.Any("state", response.PipelineRun.Status.State))
	return response.PipelineRun, nil
}
