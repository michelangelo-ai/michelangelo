package pipelinerunutils

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	ImageBuildOutputKey = "image"

	ImageBuildStepName      = "Image Build"
	ExecuteWorkflowStepName = "Execute Workflow"
	SourcePipelineStepName  = "Source Pipeline"

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

// GetPipelineRun gets a PipelineRun by the provided resource identifier.
func GetPipelineRun(ctx context.Context, pipelineRunID *apipb.ResourceIdentifier, apiHandler api.Handler,
	pipelineRun *v2.PipelineRun,
) error {
	if pipelineRunID == nil {
		return fmt.Errorf("PipelineRun resource identifier is nil")
	}

	err := apiHandler.Get(ctx, pipelineRunID.Namespace, pipelineRunID.Name, &metav1.GetOptions{}, pipelineRun)
	if err != nil {
		return fmt.Errorf("Failed to get PipelineRun namespace: %s, name: %s",
			pipelineRunID.Namespace, pipelineRunID.Name)
	}
	return nil
}
