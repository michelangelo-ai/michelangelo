package pipelinerunutils

import v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"

const (
	ImageBuildStepName                 = "Image Build"
	ImageBuildOutputKey                = "image"
	ExecuteWorkflowStepName            = "Execute Workflow"
	UniflowTaskProgressQueryHandlerKey = "task_progress"
	ImageIDAnnotationKey               = "michelangelo/uniflow-image"
)

// UniflowTaskStates
const (
	UniflowTaskStateRunning   = "RUNNING"
	UniflowTaskStatePending   = "PENDING"
	UniflowTaskStateSucceeded = "SUCCEEDED"
	UniflowTaskStateFailed    = "FAILED"
	UniflowTaskStateKilled    = "KILLED"
	UniflowTaskStateSkipped   = "SKIPPED"
)

func GetStep(pipelineRun *v2.PipelineRun, name string) *v2.PipelineRunStepInfo {
	for _, step := range pipelineRun.Status.Steps {
		if step.Name == name {
			return step
		}
	}
	return nil
}
