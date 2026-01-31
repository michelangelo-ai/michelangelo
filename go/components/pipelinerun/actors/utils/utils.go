// Package pipelinerunutils provides utility functions and constants for pipeline run actors.
//
// This package contains shared constants, step names, and helper functions used
// by the various ConditionActors that implement pipeline execution stages.
package pipelinerunutils

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// ImageBuildOutputKey is the key used in step output for the built image ID.
	ImageBuildOutputKey = "image"

	// Pipeline run step names used across actors.
	ImageBuildStepName      = "Image Build"
	ExecuteWorkflowStepName = "Execute Workflow"
	SourcePipelineStepName  = "Source Pipeline"

	// UniflowTaskProgressQueryHandlerKey is the query key for retrieving task progress from workflows.
	UniflowTaskProgressQueryHandlerKey = "task_progress"

	// ImageIDAnnotationKey is the annotation key where the Uniflow image ID is stored.
	ImageIDAnnotationKey = "michelangelo/uniflow-image"
)

// Uniflow task state constants map to workflow task execution states.
const (
	UniflowTaskStateRunning   = "RUNNING"
	UniflowTaskStatePending   = "PENDING"
	UniflowTaskStateSucceeded = "SUCCEEDED"
	UniflowTaskStateFailed    = "FAILED"
	UniflowTaskStateKilled    = "KILLED"
	UniflowTaskStateSkipped   = "SKIPPED"
)

// GetStep retrieves a specific step from a pipeline run's status by name.
//
// Returns the matching PipelineRunStepInfo or nil if not found.
func GetStep(pipelineRun *v2.PipelineRun, name string) *v2.PipelineRunStepInfo {
	for _, step := range pipelineRun.Status.Steps {
		if step.Name == name {
			return step
		}
	}
	return nil
}

// GetPipelineRun retrieves a PipelineRun resource using the provided identifier.
//
// The function fetches the PipelineRun from Kubernetes and populates the provided
// pipelineRun pointer with the result.
//
// Returns an error if the resource identifier is nil or the Get operation fails.
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
