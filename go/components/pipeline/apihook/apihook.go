// Package apihook provides the API hook that stamps a Pipeline's own type
// onto itself as the michelangelo/SourcePipelineType label, so the type can
// be queried with a label selector.
package apihook

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RegisterPipelineAPIHook registers the API hook that stamps a Pipeline's own
// Spec.Type onto itself as the michelangelo/SourcePipelineType label at
// create and update time. This mirrors the label PipelineRun/TriggerRun
// already inherit from their source Pipeline (see
// go/components/pipelinerun/apihook and go/components/triggerrun/apihook),
// so all three resource kinds can be filtered by pipeline type with the same
// label selector instead of a field selector — field selectors (parsed via
// k8s.io/apimachinery/pkg/fields) never support the set-based `in (...)`
// syntax the UI needs; only label selectors do.
func RegisterPipelineAPIHook(logger *zap.Logger) {
	v2.RegisterPipelineAPIHook(apiHook{logger: logger})
}

type apiHook struct {
	v2.NoopPipelineAPIHook
	logger *zap.Logger
}

func (a apiHook) BeforeCreate(_ context.Context, request *v2.CreatePipelineRequest) error {
	stampType(request.Pipeline)
	return nil
}

func (a apiHook) BeforeUpdate(_ context.Context, request *v2.UpdatePipelineRequest) error {
	stampType(request.Pipeline)
	return nil
}

// stampType stamps pipeline.Spec.Type onto pipeline's own
// michelangelo/SourcePipelineType label. No-op if the type is unset.
func stampType(pipeline *v2.Pipeline) {
	if pipeline == nil {
		return
	}
	if pipelineType := pipeline.Spec.GetType(); pipelineType != v2.PIPELINE_TYPE_INVALID {
		api.StampSourcePipelineTypeLabelOnCreate(pipeline, pipelineType.String())
	}
}
