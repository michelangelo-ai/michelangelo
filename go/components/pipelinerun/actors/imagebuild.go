package actors

import (
	"context"
	"fmt"

	pbtypes "github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	// ImageBuildType is the condition type for the image build stage.
	ImageBuildType = "Image Build"
)

// ImageBuildActor implements the image resolution stage of pipeline execution.
//
// This actor retrieves the container image ID from the source pipeline's annotations
// and makes it available to the workflow execution stage. The image ID is expected
// to be stored in the pipeline's "michelangelo/uniflow-image" annotation.
//
// The actor updates the pipeline run step with the resolved image ID in its output,
// which is later used by the ExecuteWorkflowActor to configure task execution.
type ImageBuildActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	logger *zap.Logger
}

// NewImageBuildActor creates a new ImageBuildActor with the specified logger.
func NewImageBuildActor(logger *zap.Logger) *ImageBuildActor {
	return &ImageBuildActor{
		logger: logger.With(zap.String("actor", "imagebuild")),
	}
}

var _ conditionInterfaces.ConditionActor[*v2.PipelineRun] = &ImageBuildActor{}

// Retrieve checks if the image build step has completed or if prerequisites are met.
//
// Returns TRUE if the image is already resolved, FALSE if the step needs to run,
// or an error condition if the source pipeline or image annotation is missing.
func (a *ImageBuildActor) Retrieve(ctx context.Context, resource *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)))

	// Check if image build step is already in a terminal state
	imageBuildStep := pipelinerunutils.GetStep(resource, pipelinerunutils.ImageBuildStepName)
	if imageBuildStep != nil {
		switch imageBuildStep.State {
		case v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED:
			logger.Info("image build already completed successfully")
			return &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_TRUE,
			}, nil
		case v2.PIPELINE_RUN_STEP_STATE_FAILED:
			logger.Info("image build failed")
			return &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, nil
		}
	}

	// Check if source pipeline is available to get image ID
	sourcePipeline := resource.Status.SourcePipeline
	if sourcePipeline == nil || sourcePipeline.Pipeline == nil {
		logger.Info("source pipeline not available yet")
		return &apipb.Condition{
			Type:   ImageBuildType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Check if image ID annotation exists
	annotations := sourcePipeline.Pipeline.Annotations
	if annotations == nil || annotations[pipelinerunutils.ImageIDAnnotationKey] == "" {
		logger.Info("image ID annotation not found in source pipeline")
		return &apipb.Condition{
			Type:    ImageBuildType,
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "Missing image ID",
			Message: fmt.Sprintf("Source pipeline is available but missing %s annotation", pipelinerunutils.ImageIDAnnotationKey),
		}, nil
	}

	// Image ID is available but build step hasn't been updated yet
	logger.Info("image ID found, image build ready to be processed")
	return &apipb.Condition{
		Type:   ImageBuildType,
		Status: apipb.CONDITION_STATUS_FALSE,
	}, nil
}

// Run retrieves the image ID from the source pipeline and updates the step status.
//
// The actor extracts the image ID from the source pipeline's annotations and
// stores it in the step's output. This image ID is used by ExecuteWorkflowActor
// to configure the container environment for task execution.
//
// Returns TRUE condition if successful, FALSE if the image ID is missing or invalid.
func (a *ImageBuildActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))

	imageBuildStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ImageBuildStepName)
	if imageBuildStep == nil {
		logger.Info("image build step not found, setting to pending")
		imageBuildStep = &v2.PipelineRunStepInfo{
			Name:        pipelinerunutils.ImageBuildStepName,
			DisplayName: pipelinerunutils.ImageBuildStepName,
			State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
			StartTime:   pbtypes.TimestampNow(),
		}
		pipelineRun.Status.Steps = append(pipelineRun.Status.Steps, imageBuildStep)
	}

	// At the moment, the image id is configured as an annotation of the source pipeline.
	// We need to get the source pipeline and check if the image id is set.

	sourcePipeline := pipelineRun.Status.SourcePipeline
	if sourcePipeline == nil || sourcePipeline.Pipeline == nil {
		logger.Info("source pipeline is not populated yet, setting to pending")
		return previousCondition, nil
	}

	annotations := sourcePipeline.Pipeline.Annotations
	if annotations == nil || annotations[pipelinerunutils.ImageIDAnnotationKey] == "" {
		logger.Info("source pipeline has no image id, setting to false")
		imageBuildStep.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		imageBuildStep.EndTime = pbtypes.TimestampNow()
		imageBuildStep.Message = fmt.Sprintf("%s not found in source pipeline annotations", pipelinerunutils.ImageIDAnnotationKey)
		return &apipb.Condition{
			Type:   ImageBuildType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	logger.Info("source pipeline has image id, setting to true")
	imageID := annotations[pipelinerunutils.ImageIDAnnotationKey]
	imageBuildStep.State = v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED
	imageBuildStep.EndTime = pbtypes.TimestampNow()
	// add the image id to the output of the step
	imageBuildStep.Output = &pbtypes.Struct{
		Fields: map[string]*pbtypes.Value{
			pipelinerunutils.ImageBuildOutputKey: {
				Kind: &pbtypes.Value_StringValue{
					StringValue: imageID,
				},
			},
		},
	}
	return &apipb.Condition{
		Type:   ImageBuildType,
		Status: apipb.CONDITION_STATUS_TRUE,
	}, nil
}

// GetType returns the condition type identifier for this actor.
func (a *ImageBuildActor) GetType() string {
	return ImageBuildType
}
