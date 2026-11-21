// Package apihook validates DECLARATIVE pipelines on create and update and
// pins each one to an interpreter release.
//
// The hook passes every other manifest type through unchanged. When the
// apiserver.declarative flag is off, a DECLARATIVE write fails with
// FailedPrecondition.
package apihook

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
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
	if req.Pipeline.Spec.Manifest.InterpreterPin != nil {
		return status.Error(codes.InvalidArgument, "interpreter_pin is server-owned")
	}
	d := a.d.defaultRelease
	return apply(req.Pipeline, d, pinOf(d), req.CreateOptions != nil && isDryRun(req.CreateOptions.DryRun))
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
	if p.Spec.Manifest.UniflowTar != "" {
		return false, status.Error(codes.InvalidArgument, "uniflow_tar is server-owned for declarative pipelines")
	}
	return false, nil
}

// apply validates p against rel and stamps pin. On a dry run it sets the
// explain-plan annotation. Otherwise it removes the annotation, so a client
// cannot store a plan.
func apply(p *v2.Pipeline, rel *declarative.Release, pin *v2.InterpreterPin, dryRun bool) error {
	w, err := declarative.Decode(p.Spec.Manifest)
	if err != nil {
		return toStatus(err)
	}
	plan, err := declarative.Plan(w, declarative.DefaultLimits(), rel, nil)
	if err != nil {
		return toStatus(err)
	}
	p.Spec.Manifest.InterpreterPin = pin
	if !dryRun {
		delete(p.Annotations, explainPlanAnnotation)
		return nil
	}
	encoded, err := (&jsonpb.Marshaler{}).MarshalToString(plan)
	if err != nil {
		return status.Errorf(codes.Internal, "encode explain plan: %v", err)
	}
	if p.Annotations == nil {
		p.Annotations = map[string]string{}
	}
	p.Annotations[explainPlanAnnotation] = encoded
	return nil
}

// toStatus maps a problem in the workflow content to InvalidArgument and any
// other error to Internal.
func toStatus(err error) error {
	var derr *declarative.Error
	if errors.As(err, &derr) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Errorf(codes.Internal, "plan declarative pipeline: %v", err)
}

func pinOf(r *declarative.Release) *v2.InterpreterPin {
	return &v2.InterpreterPin{Version: r.Version, Artifact: r.Artifact, Digest: r.Digest}
}

// isDryRun reports whether the request options ask for a dry run. The
// apiserver accepts only metav1.DryRunAll.
func isDryRun(dryRun []string) bool {
	for _, v := range dryRun {
		if v == metav1.DryRunAll {
			return true
		}
	}
	return false
}
