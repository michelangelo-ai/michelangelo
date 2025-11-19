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
	ImageBuildType = "Image Build"
)

// ImageBuildActor handles the image building stage of pipeline execution.
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

func (a *ImageBuildActor) GetType() string {
	return ImageBuildType
}
