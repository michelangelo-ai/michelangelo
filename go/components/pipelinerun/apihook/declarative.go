package apihook

import (
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelineapihook "github.com/michelangelo-ai/michelangelo/go/components/pipeline/apihook"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// checkDeclarativeRun validates a run of a DECLARATIVE pipeline spec at
// trigger. It returns nil for any other spec. revision is the name of the
// revision that the run names explicitly, or "".
func checkDeclarativeRun(run *v2.PipelineRun, spec *v2.PipelineSpec, revision string, d *pipelineapihook.Declarative) error {
	if spec.GetManifest().GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE {
		return nil
	}
	// The SourcePipeline actor reads a pipeline by name from the run namespace.
	if run.Spec.PipelineSpec == nil {
		if ns := run.Spec.GetPipeline().GetNamespace(); ns != "" && ns != run.Namespace {
			return status.Errorf(codes.InvalidArgument, "declarative pipeline run must name a pipeline in its own namespace %q, not %q", run.Namespace, ns)
		}
	}
	if !d.Enabled() {
		return status.Error(codes.FailedPrecondition, pipelineapihook.NotEnabledMessage)
	}
	rel, err := pinnedRelease(spec.Manifest.InterpreterPin, revision, d)
	if err != nil {
		return err
	}
	w, err := declarative.Decode(spec.Manifest)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err := declarative.ValidateAgainst(w, declarative.DefaultLimits(), rel); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err := declarative.CheckRunInput(run.Spec.Input); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	names := make([]string, 0, len(w.Parameters))
	for name := range w.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if w.Parameters[name].GetDefault() == nil {
			return status.Errorf(codes.InvalidArgument, "parameter %s is required; parameter overrides are not supported yet", name)
		}
	}
	return nil
}

// pinnedRelease returns the loaded release that pin names. A re-apply cannot
// change the pin of an explicit revision, so its errors ask for a new revision.
func pinnedRelease(pin *v2.InterpreterPin, revision string, d *pipelineapihook.Declarative) (*declarative.Release, error) {
	if pin.GetVersion() == "" {
		return nil, status.Error(codes.FailedPrecondition, "declarative pipeline has no interpreter pin; re-apply the pipeline through the API")
	}
	rel, ok := d.Release(pin.Version)
	switch {
	case !ok && revision != "":
		return nil, status.Errorf(codes.FailedPrecondition, "revision %q pins interpreter %q, which is not loaded on this apiserver; %s", revision, pin.Version, newRevisionHint)
	case !ok:
		return nil, status.Errorf(codes.FailedPrecondition, "interpreter %q is not loaded on this apiserver; loaded releases: [%s]", pin.Version, strings.Join(d.Versions(), ", "))
	case (pin.Artifact != rel.Artifact || pin.Digest != rel.Digest) && revision != "":
		return nil, status.Errorf(codes.FailedPrecondition, "revision %q pins interpreter %q, which does not equal the loaded release; %s", revision, pin.Version, newRevisionHint)
	case pin.Artifact != rel.Artifact || pin.Digest != rel.Digest:
		return nil, status.Errorf(codes.FailedPrecondition, "interpreter pin %q does not equal the loaded release; re-apply the pipeline through the API", pin.Version)
	}
	return rel, nil
}

const newRevisionHint = "apply the pipeline from a new commit to create a new revision"

// takeLivePin copies the interpreter pin and uniflow_tar of the live Pipeline
// into a DECLARATIVE spec that a run resolved from status.latestRevision. A
// re-apply on the same commit changes the pin of the live Pipeline but keeps
// the revision, so the snapshot pin can name a release that is gone.
func takeLivePin(spec *v2.PipelineSpec, live *v2.Pipeline) {
	if spec.GetManifest().GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE ||
		live.Spec.GetManifest().GetType() != v2.PIPELINE_MANIFEST_TYPE_DECLARATIVE {
		return
	}
	spec.Manifest.InterpreterPin = nil
	if pin := live.Spec.Manifest.InterpreterPin; pin != nil {
		clone := *pin
		spec.Manifest.InterpreterPin = &clone
	}
	spec.Manifest.UniflowTar = live.Spec.Manifest.UniflowTar
}
