package actors

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	SourcePipelineType = "SourcePipeline"
)

type SourcePipelineActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	apiHandler api.Handler
	logger     *zap.Logger
}

func NewSourcePipelineActor(apiHandler api.Handler, logger *zap.Logger) *SourcePipelineActor {
	return &SourcePipelineActor{
		apiHandler: apiHandler,
		logger:     logger.With(zap.String("actor", "sourcepipeline")),
	}
}

var _ conditionInterfaces.ConditionActor[*v2.PipelineRun] = &SourcePipelineActor{}

func (a *SourcePipelineActor) Retrieve(ctx context.Context, resource *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)))

	// Check if source pipeline is already populated
	if resource.Status.SourcePipeline != nil && resource.Status.SourcePipeline.Pipeline != nil {
		logger.Info("source pipeline already retrieved")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_TRUE,
		}, nil
	}

	// Check if this is a DevRun with inline pipeline_spec
	if resource.Spec.GetPipelineSpec() != nil {
		logger.Info("dev run detected with inline pipeline_spec")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Check if regular pipeline run has pipeline reference
	if resource.Spec.GetPipeline() == nil {
		logger.Info("pipeline run has no pipeline resource ID")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Try to fetch the pipeline to verify it exists
	pipeline := &v2.Pipeline{}
	pipelineResourceID := resource.Spec.Pipeline
	err := a.apiHandler.Get(ctx, resource.Namespace, pipelineResourceID.GetName(), &metav1.GetOptions{}, pipeline)
	if err != nil {
		logger.Error("failed to retrieve pipeline", zap.Error(err))
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Pipeline exists but hasn't been loaded into status yet
	logger.Info("pipeline exists but needs to be loaded into status")
	return &apipb.Condition{
		Type:   SourcePipelineType,
		Status: apipb.CONDITION_STATUS_FALSE,
	}, nil
}

func (a *SourcePipelineActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	if previousCondition == nil {
		logger.Info("pipeline run has no previous condition, setting to unknown")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_UNKNOWN,
		}, nil
	}

	if previousCondition.Status != apipb.CONDITION_STATUS_UNKNOWN {
		// the previous condition is terminal, so we don't need to run the actor again
		logger.Info("pipeline run has a terminal condition, skipping")
		return previousCondition, nil
	}

	pipelineRunSpec := pipelineRun.GetSpec()

	// Check if this is a DevRun (pipeline_spec field is populated)
	pipeline := &v2.Pipeline{}
	if pipelineRunSpec.GetPipelineSpec() != nil {
		logger.Info("dev run detected, creating pipeline from inline pipeline_spec")

		// Create pipeline from inline spec
		devPipeline, err := a.createPipelineFromSpec(pipelineRunSpec, pipelineRun)
		if err != nil {
			logger.Error("failed to create pipeline from spec", zap.Error(err))
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to create pipeline from spec: %w", err)
		}
		pipeline = devPipeline
	} else {
		// Regular pipeline run - fetch from Kubernetes using pipeline reference
		if pipelineRunSpec.GetPipeline() == nil {
			logger.Info("pipeline run has no pipeline resource ID, setting to false")
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("pipeline resource ID is nil")
		}

		pipelineResourceID := pipelineRunSpec.GetPipeline()
		err := a.apiHandler.Get(ctx, pipelineRun.Namespace, pipelineResourceID.GetName(), &metav1.GetOptions{}, pipeline)
		if err != nil {
			logger.Error("failed to get pipeline", zap.Error(err))
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to get pipeline: %w", err)
		}

		pipeline.ObjectMeta = metav1.ObjectMeta{
			Name:        pipeline.Name,
			Namespace:   pipeline.Namespace,
			Labels:      pipeline.Labels,
			Annotations: pipeline.Annotations,
		}
	}
	pipelineRun.Status.SourcePipeline = &v2.SourcePipeline{
		Pipeline: pipeline,
	}

	logger.Info("pipeline run has a pipeline resource, setting to true")
	return &apipb.Condition{
		Type:   SourcePipelineType,
		Status: apipb.CONDITION_STATUS_TRUE,
	}, nil
}

func (a *SourcePipelineActor) GetType() string {
	return SourcePipelineType
}

// createPipelineFromSpec creates a Pipeline CR from inline PipelineSpec for dev runs
func (a *SourcePipelineActor) createPipelineFromSpec(pipelineRunSpec v2.PipelineRunSpec, pipelineRun *v2.PipelineRun) (*v2.Pipeline, error) {
	pipelineSpec := pipelineRunSpec.GetPipelineSpec()

	// Create pipeline CR with metadata from pipeline run
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("devrun-%s", pipelineRun.Name),
			Namespace:   pipelineRun.Namespace,
			Annotations: make(map[string]string),
		},
		Spec: *pipelineSpec, // Use inline spec directly
	}

	// Copy annotations from pipeline run metadata (set by CLI during dev run creation)
	// For dev runs, annotations like image-id are stored in the PipelineRun metadata
	for k, v := range pipelineRun.Annotations {
		pipeline.ObjectMeta.Annotations[k] = v
	}

	// Note: Environment variable overrides for DevRuns are applied in ExecuteWorkflow actor
	// during workflow input preparation, preserving the original pipeline manifest

	return pipeline, nil
}
