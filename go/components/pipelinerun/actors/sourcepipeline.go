package actors

import (
	"context"
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		logger.Info("previous condition", zap.Any("previousCondition", previousCondition))
		return previousCondition, nil
	}

	pipelineRunSpec := pipelineRun.GetSpec()
	if pipelineRunSpec.GetPipeline() == nil {
		logger.Info("pipeline run has no pipeline resource ID, setting to false")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, fmt.Errorf("pipeline resource ID is nil")
	}

	pipelineResourceID := pipelineRunSpec.GetPipeline()
	pipeline := &v2.Pipeline{}
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
