// Package apihook validates DECLARATIVE pipelines on create and update and
// pins each one to an interpreter release.
//
// The hook passes every other manifest type through, unless the request sets
// interpreter_pin. It removes only the explain-plan annotation. When the apiserver.declarative flag is off, a
// DECLARATIVE write fails with FailedPrecondition.
package apihook

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
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
		return status.Error(codes.InvalidArgument, "interpreter_pin is server-owned; remove interpreter_pin from the request")
	}
	d := a.d.defaultRelease
	pin := pinOf(d)
	if err := checkTar(req.Pipeline.Spec.Manifest.UniflowTar, pin, ""); err != nil {
		return err
	}
	return apply(req.Pipeline, d, pin, req.CreateOptions != nil && isDryRun(req.CreateOptions.DryRun))
}

// BeforeUpdate validates a DECLARATIVE pipeline and keeps or moves its pin.
func (a apiHook) BeforeUpdate(ctx context.Context, req *v2.UpdatePipelineRequest) error {
	if skip, err := a.gate(req.Pipeline); skip || err != nil {
		return err
	}
	p := req.Pipeline
	stored := &v2.Pipeline{}
	if err := a.apiHandler.Get(ctx, p.Namespace, p.Name, &metav1.GetOptions{}, stored); err != nil {
		if utils.IsNotFoundError(err) {
			// The Update call that follows reports the missing pipeline.
			return nil
		}
		return err
	}
	rel, pin, err := a.target(stored, p.Spec.Manifest.InterpreterPin)
	if err != nil {
		return err
	}
	var storedArtifact string
	if stored.Spec.Manifest.GetType() == v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE {
		storedArtifact = stored.Spec.Manifest.GetInterpreterPin().GetArtifact()
	}
	if err := checkTar(p.Spec.Manifest.UniflowTar, pin, storedArtifact); err != nil {
		return err
	}
	return apply(p, rel, pin, req.UpdateOptions != nil && isDryRun(req.UpdateOptions.DryRun))
}

// target resolves the release and the pin of an update. A stored pipeline
// that is not a pinned DECLARATIVE pipeline follows the create rules. This
// covers a type change and a pipeline that kubectl wrote without a pin.
//
// A request artifact or digest is accepted when it equals the pin that the
// hook writes, or the stored pin. The second case is an echo from a
// get-modify-update, and the hook replaces it.
func (a apiHook) target(stored *v2.Pipeline, reqPin *v2.InterpreterPin) (*declarative.Release, *v2.InterpreterPin, error) {
	storedPin := stored.Spec.Manifest.GetInterpreterPin()
	if stored.Spec.Manifest.GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE || storedPin == nil {
		if reqPin != nil {
			return nil, nil, status.Error(codes.InvalidArgument, "interpreter_pin is server-owned; remove interpreter_pin from the request")
		}
		return a.d.defaultRelease, pinOf(a.d.defaultRelease), nil
	}

	var rel *declarative.Release
	var pin *v2.InterpreterPin
	if v := reqPin.GetVersion(); v == "" || v == storedPin.Version {
		r, ok := a.d.releases[storedPin.Version]
		if !ok {
			return nil, nil, status.Errorf(codes.InvalidArgument,
				"interpreter %s is no longer available; set interpreter_pin.version to one of %s",
				storedPin.Version, a.d.versionList())
		}
		switch storedPin.Digest {
		case r.Digest:
			rel, pin = r, storedPin
		case "":
			// kubectl wrote a pin with no digest. Stamp it again from the
			// release of the same version.
			rel, pin = r, pinOf(r)
		default:
			return nil, nil, status.Errorf(codes.FailedPrecondition,
				"the release file for interpreter %s changed after this pipeline was pinned; set interpreter_pin.version to another release in %s",
				storedPin.Version, a.d.versionList())
		}
	} else {
		r, ok := a.d.releases[v]
		if !ok {
			return nil, nil, status.Errorf(codes.InvalidArgument,
				"interpreter_pin.version %q is not a known release; use one of %s", v, a.d.versionList())
		}
		rel, pin = r, pinOf(r)
	}

	if s := reqPin.GetArtifact(); s != "" && s != pin.Artifact && s != storedPin.Artifact {
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"interpreter_pin.artifact %q does not match the pin for interpreter %s; leave it unset", s, rel.Version)
	}
	if s := reqPin.GetDigest(); s != "" && s != pin.Digest && s != storedPin.Digest {
		return nil, nil, status.Errorf(codes.InvalidArgument,
			"interpreter_pin.digest %q does not match the pin for interpreter %s; leave it unset", s, rel.Version)
	}
	return rel, pin, nil
}

// gate reports skip for a pipeline that is not DECLARATIVE. On skip, it
// removes the explain-plan annotation, so a client cannot store a plan on
// that pipeline. It returns an error for a pipeline that is not DECLARATIVE
// but carries interpreter_pin, and for a DECLARATIVE pipeline when the flag
// is off. The pin check does not depend on the flag.
func (a apiHook) gate(p *v2.Pipeline) (skip bool, err error) {
	if p.Spec.Manifest.GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE {
		if p.Spec.Manifest.GetInterpreterPin() != nil {
			return false, status.Error(codes.InvalidArgument, "interpreter_pin is only valid for declarative pipelines")
		}
		delete(p.Annotations, explainPlanAnnotation)
		return true, nil
	}
	if !a.d.enabled {
		return false, status.Error(codes.FailedPrecondition, "declarative pipelines are not enabled on this apiserver; an operator must set apiserver.declarative.enabled=true")
	}
	return false, nil
}

// checkTar accepts a client uniflow_tar only when it equals the artifact of
// pin, so a get-modify-update round trip succeeds. On an update, it also
// accepts storedArtifact, the artifact of the stored pin. A client that
// changes only interpreter_pin.version echoes that value, and the hook
// replaces it.
func checkTar(tar string, pin *v2.InterpreterPin, storedArtifact string) error {
	if tar == "" || tar == pin.Artifact || (storedArtifact != "" && tar == storedArtifact) {
		return nil
	}
	return status.Error(codes.InvalidArgument, "uniflow_tar is server-owned for declarative pipelines; remove uniflow_tar from the request")
}

// apply validates p against rel, stamps pin, and sets uniflow_tar to the pin
// artifact. On a dry run it sets the
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
	p.Spec.Manifest.UniflowTar = pin.Artifact
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
