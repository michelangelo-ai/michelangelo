package trigger

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Activities = (*activities)(nil)

// activities struct encapsulates the YARPC clients for pipeline run services.
type activities struct {
	pipelineRunService v2pb.PipelineRunServiceYARPCClient
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
	logger.Info("create pipeline run activity started",
		zap.String("operation", "create_pipeline_run"),
		zap.String("pipeline", request.PipelineRun.Spec.Pipeline.Name),
		zap.String("namespace", request.PipelineRun.Namespace))

	response, err := r.pipelineRunService.CreatePipelineRun(ctx, request)
	if err != nil || response == nil || response.PipelineRun == nil {
		logger.Error("failed to create pipeline run",
			zap.String("operation", "create_pipeline_run"),
			zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "CreatePipelineRun", err.Error())
	}

	logger.Info("pipeline run created successfully",
		zap.String("operation", "create_pipeline_run"),
		zap.String("pipeline_run_name", response.PipelineRun.Name))
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
	triggerType := triggerrun.GetTriggerType(triggerRun)
	logger.Info("generate batch run params activity started",
		zap.String("operation", "generate_batch_params"),
		zap.String("trigger_run", triggerRun.Name),
		zap.String("trigger_type", triggerType))

	// Get appropriate parameter generator for this trigger type
	generator := parameter.GetParameterGenerator(triggerType)

	// Use interface method to generate parameters
	batches, err := generator.GenerateBatchParams(triggerRun)
	if err != nil {
		logger.Error("failed to generate batch params",
			zap.String("operation", "generate_batch_params"),
			zap.String("trigger_run", triggerRun.Name),
			zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GenerateBatchParams", err.Error())
	}

	logger.Info("batch params generated successfully",
		zap.String("operation", "generate_batch_params"),
		zap.Int("batch_count", len(batches)),
		zap.String("trigger_type", triggerType))
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
	triggerType := triggerrun.GetTriggerType(triggerRun)
	logger.Info("generate concurrent run params activity started",
		zap.String("operation", "generate_concurrent_params"),
		zap.String("trigger_run", triggerRun.Name),
		zap.String("trigger_type", triggerType))

	// Get appropriate parameter generator for this trigger type
	generator := parameter.GetParameterGenerator(triggerType)

	// Use interface method to generate parameters
	params, err := generator.GenerateConcurrentParams(triggerRun)
	if err != nil {
		logger.Error("failed to generate concurrent params",
			zap.String("operation", "generate_concurrent_params"),
			zap.String("trigger_run", triggerRun.Name),
			zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GenerateConcurrentParams", err.Error())
	}

	logger.Info("concurrent params generated successfully",
		zap.String("operation", "generate_concurrent_params"),
		zap.Int("param_count", len(params)),
		zap.String("trigger_type", triggerType))
	return params, nil
}

// PipelineRunSensor monitors pipeline run status.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing the pipeline run name and namespace.
//
// Returns:
// - *v2pb.PipelineRun: The updated pipeline run status.
// - error: Error information if the operation fails.
func (r *activities) PipelineRunSensor(ctx context.Context, request *v2pb.GetPipelineRunRequest) (*v2pb.PipelineRun, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("pipeline run sensor activity started",
		zap.String("operation", "pipeline_run_sensor"),
		zap.String("pipeline_run", request.Name),
		zap.String("namespace", request.Namespace))

	response, err := r.pipelineRunService.GetPipelineRun(ctx, request)
	if err != nil {
		logger.Error("failed to get pipeline run status",
			zap.String("operation", "pipeline_run_sensor"),
			zap.String("pipeline_run", request.Name),
			zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "GetPipelineRun", err.Error())
	}

	if response == nil || response.PipelineRun == nil {
		err := fmt.Errorf("empty response from pipeline run service")
		logger.Error("empty response from pipeline run service",
			zap.String("operation", "pipeline_run_sensor"),
			zap.String("pipeline_run", request.Name),
			zap.Error(err))
		return nil, workflow.NewCustomError(ctx, "EmptyResponse", err.Error())
	}
	logger.Info("pipeline run status retrieved successfully",
		zap.String("operation", "pipeline_run_sensor"),
		zap.String("pipeline_run", request.Name),
		zap.String("state", response.PipelineRun.Status.State.String()))
	response.PipelineRun = cropPipelineRun(response.PipelineRun)
	switch response.PipelineRun.Status.State {
	case v2pb.PIPELINE_RUN_STATE_INVALID, v2pb.PIPELINE_RUN_STATE_PENDING, v2pb.PIPELINE_RUN_STATE_RUNNING:
		logger.Error("pipeline run is in the non-terminal status",
			zap.String("operation", "pipeline_run_sensor"),
			zap.String("pipeline_run", request.Name),
			zap.String("state", response.PipelineRun.Status.State.String()))
		return nil, workflow.NewCustomError(ctx, "PipelineRunNotReady", fmt.Sprintf("PipelineRun is in the non-terminal status: %v", response.PipelineRun.Status.State.String()))
	}
	return response.PipelineRun, nil
}

// cropPipelineRun reduces the size of a PipelineRun object to prevent workflow response size limit errors.
//
// Cadence/Temporal workflows have a maximum response size limit (typically 2MB). When monitoring
// pipeline runs with large input data or extensive status information, the full PipelineRun object
// can exceed this limit and cause workflow failures.
//
// This function creates a trimmed copy that includes only the essential fields needed for trigger
// workflow logic:
//
// Preserved fields:
//   - TypeMeta: API version and kind information
//   - ObjectMeta: Namespace, name, labels, annotations (for identification and metadata)
//   - Spec: Pipeline specification (with input data removed)
//   - Status: Essential status fields (state, log URL, error message, code, end time)
//
// Removed fields:
//   - Spec.Input: Large input parameters that may contain datasets or configuration blobs
//   - Status fields: Detailed execution history, metrics, and other verbose status information
//     (only keeps State, LogUrl, ErrorMessage, Code, and EndTime)
//
// Params:
//   - pr: The original PipelineRun object to crop
//
// Returns:
//   - *v2pb.PipelineRun: A cropped copy with reduced size, or nil if input is nil
func cropPipelineRun(pr *v2pb.PipelineRun) *v2pb.PipelineRun {
	if pr == nil {
		return nil
	}
	status := pr.Status
	res := &v2pb.PipelineRun{
		TypeMeta: pr.TypeMeta,
		ObjectMeta: v1.ObjectMeta{
			Namespace:   pr.Namespace,
			Name:        pr.Name,
			Labels:      pr.Labels,
			Annotations: pr.Annotations,
		},
		Spec: pr.Spec,
		Status: v2pb.PipelineRunStatus{
			State:        status.State,
			LogUrl:       status.LogUrl,
			ErrorMessage: status.ErrorMessage,
			Code:         status.Code,
			EndTime:      status.EndTime,
		},
	}
	// Remove input data to reduce size
	res.Spec.Input = nil
	return res
}
