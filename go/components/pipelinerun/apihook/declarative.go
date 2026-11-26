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
// trigger. It returns nil for any other spec.
func checkDeclarativeRun(run *v2.PipelineRun, spec *v2.PipelineSpec, d *pipelineapihook.Declarative) error {
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
	rel, err := pinnedRelease(spec.Manifest.InterpreterPin, d)
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

func pinnedRelease(pin *v2.InterpreterPin, d *pipelineapihook.Declarative) (*declarative.Release, error) {
	if pin.GetVersion() == "" {
		return nil, status.Error(codes.FailedPrecondition, "declarative pipeline has no interpreter pin; re-apply the pipeline through the API")
	}
	rel, ok := d.Release(pin.Version)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "interpreter %q is not loaded on this apiserver; loaded releases: [%s]", pin.Version, strings.Join(d.Versions(), ", "))
	}
	if pin.Artifact != rel.Artifact || pin.Digest != rel.Digest {
		return nil, status.Errorf(codes.FailedPrecondition, "interpreter pin %q does not equal the loaded release; re-apply the pipeline through the API", pin.Version)
	}
	return rel, nil
}
