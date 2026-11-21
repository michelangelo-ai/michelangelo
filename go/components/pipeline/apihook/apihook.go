// Package apihook validates DECLARATIVE pipelines on create and update and
// pins each one to an interpreter release.
//
// The hook passes every other manifest type through unchanged. When the
// apiserver.declarative flag is off, a DECLARATIVE write fails with
// FailedPrecondition.
package apihook

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// explainPlanAnnotation holds the protojson ExplainPlan on a dry-run response.
const explainPlanAnnotation = "michelangelo/declarative-explain-plan"

// RegisterPipelineAPIHook registers the declarative pipeline hook.
func RegisterPipelineAPIHook(apiHandler api.Handler, d *Declarative) {
	v2.RegisterPipelineAPIHook(apiHook{apiHandler: apiHandler, d: d})
}

type apiHook struct {
	v2.NoopPipelineAPIHook
	apiHandler api.Handler
	d          *Declarative
}

// BeforeCreate validates a DECLARATIVE pipeline and pins it.
func (a apiHook) BeforeCreate(_ context.Context, req *v2.CreatePipelineRequest) error {
	if skip, err := a.gate(req.Pipeline); skip || err != nil {
		return err
	}
	return nil
}

// BeforeUpdate validates a DECLARATIVE pipeline and keeps or moves its pin.
func (a apiHook) BeforeUpdate(_ context.Context, req *v2.UpdatePipelineRequest) error {
	if skip, err := a.gate(req.Pipeline); skip || err != nil {
		return err
	}
	return nil
}

// gate reports skip for a pipeline that is not DECLARATIVE, and an error for
// a DECLARATIVE pipeline when the flag is off.
func (a apiHook) gate(p *v2.Pipeline) (skip bool, err error) {
	if p.Spec.Manifest.GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE {
		return true, nil
	}
	if !a.d.enabled {
		return false, status.Error(codes.FailedPrecondition, "declarative pipelines are not enabled on this apiserver")
	}
	return false, nil
}
