package actors

import (
	"context"
	"fmt"
	"testing"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestImageBuildActor(t *testing.T) {
	testCases := []struct {
		name                   string
		pipelineRun            *v2.PipelineRun
		expectedCondition      *apipb.Condition
		expectedImageBuildStep *v2.PipelineRunStepInfo
		errMsg                 string
	}{
		{
			name: "source pipeline is not populated yet",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        ImageBuildType,
							DisplayName: ImageBuildType,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedImageBuildStep: &v2.PipelineRunStepInfo{
				Name:        ImageBuildType,
				DisplayName: ImageBuildType,
				State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
				StartTime:   pbtypes.TimestampNow(),
				Message:     "",
			},
			errMsg: "",
		},
		{
			name: "image id is not set in source pipeline annotations",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        ImageBuildType,
							DisplayName: ImageBuildType,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedImageBuildStep: &v2.PipelineRunStepInfo{
				Name:        ImageBuildType,
				DisplayName: ImageBuildType,
				State:       v2.PIPELINE_RUN_STEP_STATE_FAILED,
				StartTime:   pbtypes.TimestampNow(),
				Message:     fmt.Sprintf("%s not found in source pipeline annotations", pipelinerunutils.ImageIDAnnotationKey),
			},
			errMsg: "",
		},
		{
			name: "image id is set in source pipeline annotations",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
								},
							},
						},
					},
					Conditions: []*apipb.Condition{
						{
							Type:   ImageBuildType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:        ImageBuildType,
							DisplayName: ImageBuildType,
							State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
							StartTime:   pbtypes.TimestampNow(),
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedImageBuildStep: &v2.PipelineRunStepInfo{
				Name:        ImageBuildType,
				DisplayName: ImageBuildType,
				State:       v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
				StartTime:   pbtypes.TimestampNow(),
				Message:     "",
				Output: &pbtypes.Struct{
					Fields: map[string]*pbtypes.Value{
						pipelinerunutils.ImageBuildOutputKey: {
							Kind: &pbtypes.Value_StringValue{StringValue: "test-image-id"},
						},
					},
				},
				EndTime: pbtypes.TimestampNow(),
			},
			errMsg: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actor := setUpImageBuildActor(t)
			previousCondition := conditionUtils.GetCondition(ImageBuildType, testCase.pipelineRun.Status.Conditions)
			imageBuildCondition, err := actor.Run(context.Background(), testCase.pipelineRun, previousCondition)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedCondition, imageBuildCondition)
				// we do not check the start and end time.
				imageBuildStep := pipelinerunutils.GetStep(testCase.pipelineRun, ImageBuildType)
				require.Equal(t, testCase.expectedImageBuildStep.State, imageBuildStep.State)
				require.Equal(t, testCase.expectedImageBuildStep.Message, imageBuildStep.Message)
				require.Equal(t, testCase.expectedImageBuildStep.Output, imageBuildStep.Output)
			}
		})
	}
}

func setUpImageBuildActor(t *testing.T) *ImageBuildActor {
	return NewImageBuildActor(zaptest.NewLogger(t))
}

func TestImageBuildActor_Retrieve(t *testing.T) {
	testCases := []struct {
		name              string
		pipelineRun       *v2.PipelineRun
		expectedCondition *apipb.Condition
	}{
		{
			name: "Image build step already succeeded",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ImageBuildStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
		{
			name: "Image build step already failed",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ImageBuildStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_FAILED,
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Source pipeline not available",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Source pipeline available but image ID annotation missing",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:    ImageBuildType,
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "Missing image ID",
				Message: fmt.Sprintf("Source pipeline is available but missing %s annotation", pipelinerunutils.ImageIDAnnotationKey),
			},
		},
		{
			name: "Image ID found but step not updated",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
								},
							},
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
		{
			name: "Image build step pending but source pipeline available",
			pipelineRun: &v2.PipelineRun{
				Status: v2.PipelineRunStatus{
					Steps: []*v2.PipelineRunStepInfo{
						{
							Name:  pipelinerunutils.ImageBuildStepName,
							State: v2.PIPELINE_RUN_STEP_STATE_PENDING,
						},
					},
					SourcePipeline: &v2.SourcePipeline{
						Pipeline: &v2.Pipeline{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									pipelinerunutils.ImageIDAnnotationKey: "test-image-id",
								},
							},
						},
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   ImageBuildType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actor := setUpImageBuildActor(t)
			condition, err := actor.Retrieve(context.Background(), testCase.pipelineRun, nil)

			require.NoError(t, err)
			require.Equal(t, testCase.expectedCondition.Type, condition.Type)
			require.Equal(t, testCase.expectedCondition.Status, condition.Status)
			if testCase.expectedCondition.Reason != "" {
				require.Equal(t, testCase.expectedCondition.Reason, condition.Reason)
			}
			if testCase.expectedCondition.Message != "" {
				require.Equal(t, testCase.expectedCondition.Message, condition.Message)
			}
		})
	}
}
