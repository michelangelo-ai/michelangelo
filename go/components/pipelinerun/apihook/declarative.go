package apihook

import (
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelineapihook "github.com/michelangelo-ai/michelangelo/go/components/pipeline/apihook"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// lastScheduleTimestampEnv is the only environ key that a declarative run
// accepts. The cron trigger sets it. Other keys can change the validated
// job_specs or the task image.
const lastScheduleTimestampEnv = "LAST_SCHEDULE_TIMESTAMP"

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
	if err := checkRunInput(run.Spec.Input); err != nil {
		return err
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

// checkRunInput rejects the run input keys that a declarative run cannot use.
// The run controller ignores them, so the check stops a run that would not
// do what its creator asked.
func checkRunInput(input *types.Struct) error {
	fields := input.GetFields()
	for _, key := range []string{"task_configs", "workflow_config"} {
		if _, ok := fields[key]; ok {
			return status.Errorf(codes.InvalidArgument, "run input %s is not supported for declarative pipelines", key)
		}
	}
	for _, key := range []string{"args", "kw_args"} {
		if !isEmpty(fields[key]) {
			return status.Errorf(codes.InvalidArgument, "run input %s must be empty for declarative pipelines", key)
		}
	}
	if !isEmpty(fields["parameter_map"]) {
		return status.Error(codes.InvalidArgument, "parameter overrides are not supported yet")
	}
	environ := fields["environ"]
	if isEmpty(environ) {
		return nil
	}
	env := environ.GetStructValue()
	if env == nil {
		return status.Error(codes.InvalidArgument, "run input environ must be a struct")
	}
	keys := make([]string, 0, len(env.Fields))
	for k := range env.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k != lastScheduleTimestampEnv {
			return status.Errorf(codes.InvalidArgument, "run input environ key %q is not supported for declarative pipelines; only %s is allowed", k, lastScheduleTimestampEnv)
		}
	}
	return nil
}

// isEmpty reports whether v is absent, null, an empty list, or an empty struct.
func isEmpty(v *types.Value) bool {
	switch k := v.GetKind().(type) {
	case nil, *types.Value_NullValue:
		return true
	case *types.Value_ListValue:
		return len(k.ListValue.GetValues()) == 0
	case *types.Value_StructValue:
		return len(k.StructValue.GetFields()) == 0
	}
	return false
}
