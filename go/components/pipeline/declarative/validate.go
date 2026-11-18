package declarative

import (
	"fmt"
	"math"
	"sort"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const maxSafeInteger = 1<<53 - 1

var typeNames = map[v2.ParameterType]string{
	v2.PARAMETER_TYPE_STRING:  "string",
	v2.PARAMETER_TYPE_INTEGER: "integer",
	v2.PARAMETER_TYPE_NUMBER:  "number",
	v2.PARAMETER_TYPE_BOOLEAN: "boolean",
	v2.PARAMETER_TYPE_LIST:    "list",
	v2.PARAMETER_TYPE_DICT:    "dict",
}

// typeName returns the schema type name of t, or "" for an unknown type.
func typeName(t v2.ParameterType) string { return typeNames[t] }

// Validate runs the checks that need no task-type registry. It returns the
// first failure.
func Validate(w *v2.DeclarativeWorkflow, lim Limits) error {
	lim = lim.withDefaults()
	if w == nil {
		return &Error{Msg: "workflow is not set"}
	}
	if w.SchemaVersion != SchemaVersion {
		return &Error{Msg: fmt.Sprintf("schema_version %q is not supported; use %q", w.SchemaVersion, SchemaVersion)}
	}
	if len(w.Tasks) == 0 {
		return &Error{Msg: "tasks must have at least one entry"}
	}
	if len(w.Tasks) > lim.MaxTasks {
		return &Error{Msg: fmt.Sprintf("tasks has %d entries; the limit is %d", len(w.Tasks), lim.MaxTasks)}
	}
	g, err := buildGraph(w)
	if err != nil {
		return err
	}
	if err := validateParameters(w); err != nil {
		return err
	}
	refs, err := collectRefs(w)
	if err != nil {
		return err
	}
	if err := checkParamRefs(w, refs); err != nil {
		return err
	}
	return checkTaskRefs(w, g, refs)
}

// validateParameters checks each parameter: name, type, exactly one of
// default or required, and a default that fits the type.
func validateParameters(w *v2.DeclarativeWorkflow) error {
	names := make([]string, 0, len(w.Parameters))
	for n := range w.Parameters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if !taskNameRe.MatchString(n) {
			return &Error{Param: n, Msg: "name must match " + taskNameRe.String()}
		}
		spec := w.Parameters[n]
		if spec == nil {
			return &Error{Param: n, Msg: "parameter is not set"}
		}
		if spec.Type == v2.PARAMETER_TYPE_UNSPECIFIED {
			return &Error{Param: n, Msg: "type is not set"}
		}
		if _, ok := typeNames[spec.Type]; !ok {
			return &Error{Param: n, Msg: fmt.Sprintf("type %d is not a known ParameterType", spec.Type)}
		}
		if (spec.Default != nil) == spec.Required {
			return &Error{Param: n, Msg: "exactly one of default or required must be set"}
		}
		if spec.Default != nil {
			if err := checkDefault(n, spec); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkDefault checks that the default value of a parameter fits its type.
func checkDefault(name string, spec *v2.ParameterSpec) error {
	d := spec.Default
	tn := typeName(spec.Type)
	if d.Kind == nil {
		return &Error{Param: name, Msg: "default must not be null"}
	}
	if _, isNull := d.Kind.(*types.Value_NullValue); isNull {
		return &Error{Param: name, Msg: "default must not be null"}
	}
	switch spec.Type {
	case v2.PARAMETER_TYPE_STRING:
		if _, ok := d.Kind.(*types.Value_StringValue); !ok {
			return &Error{Param: name, Msg: "default must be a string for type " + tn}
		}
	case v2.PARAMETER_TYPE_INTEGER:
		nv, ok := d.Kind.(*types.Value_NumberValue)
		if !ok {
			return &Error{Param: name, Msg: "default must be a number for type " + tn}
		}
		return checkIntegral(name, "default", nv.NumberValue)
	case v2.PARAMETER_TYPE_NUMBER:
		nv, ok := d.Kind.(*types.Value_NumberValue)
		if !ok {
			return &Error{Param: name, Msg: "default must be a number for type " + tn}
		}
		if math.IsInf(nv.NumberValue, 0) || math.IsNaN(nv.NumberValue) {
			return &Error{Param: name, Msg: "default must be finite"}
		}
	case v2.PARAMETER_TYPE_BOOLEAN:
		if _, ok := d.Kind.(*types.Value_BoolValue); !ok {
			return &Error{Param: name, Msg: "default must be a bool for type " + tn}
		}
	case v2.PARAMETER_TYPE_LIST:
		if _, ok := d.Kind.(*types.Value_ListValue); !ok {
			return &Error{Param: name, Msg: "default must be a list for type " + tn}
		}
	case v2.PARAMETER_TYPE_DICT:
		if _, ok := d.Kind.(*types.Value_StructValue); !ok {
			return &Error{Param: name, Msg: "default must be a struct for type " + tn}
		}
	}
	return nil
}

// checkIntegral checks that f is a whole number inside the range that a
// float64 holds exactly. what names the value in the error.
func checkIntegral(param, what string, f float64) error {
	if f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) {
		return &Error{Param: param, Msg: fmt.Sprintf("%s %v is not an integer", what, f)}
	}
	if math.Abs(f) > maxSafeInteger {
		return &Error{Param: param, Msg: fmt.Sprintf("%s %.0f is outside ±%d", what, f, int64(maxSafeInteger))}
	}
	return nil
}

// taskRef is one reference found in a task, with the field that holds it.
type taskRef struct {
	Task  string
	Field string
	Ref   ref
}

// collectRefs parses the references in the config and job_specs of each
// task and returns them by task name.
func collectRefs(w *v2.DeclarativeWorkflow) (map[string][]taskRef, error) {
	out := map[string][]taskRef{}
	names := make([]string, 0, len(w.Tasks))
	for n := range w.Tasks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := w.Tasks[n]
		var walkErr error
		walk := func(prefix string, s *types.Struct) {
			walkStruct(prefix, s, func(field string, v string) {
				if walkErr != nil {
					return
				}
				rs, err := parseRefs(v)
				if err != nil {
					walkErr = &Error{Task: n, Field: field, Msg: err.Error()}
					return
				}
				for _, r := range rs {
					out[n] = append(out[n], taskRef{Task: n, Field: field, Ref: r})
				}
			})
		}
		walk("config", t.Config)
		walk("job_specs", t.JobSpecs)
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return out, nil
}

// walkStruct calls fn for each string value in s, in sorted key order, with
// its field path.
func walkStruct(prefix string, s *types.Struct, fn func(field, value string)) {
	if s == nil {
		return
	}
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		walkValue(prefix+"."+k, s.Fields[k], fn)
	}
}

// walkValue calls fn for each string value in v and in the values nested in it.
func walkValue(path string, v *types.Value, fn func(field, value string)) {
	if v == nil {
		return
	}
	switch k := v.Kind.(type) {
	case *types.Value_StringValue:
		fn(path, k.StringValue)
	case *types.Value_StructValue:
		walkStruct(path, k.StructValue, fn)
	case *types.Value_ListValue:
		if k.ListValue == nil {
			return
		}
		for i, e := range k.ListValue.Values {
			walkValue(fmt.Sprintf("%s[%d]", path, i), e, fn)
		}
	}
}

// checkParamRefs checks that each parameter reference names a declared
// parameter, and that a list or dict parameter is not embedded in a longer string.
func checkParamRefs(w *v2.DeclarativeWorkflow, refs map[string][]taskRef) error {
	names := make([]string, 0, len(refs))
	for n := range refs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, tr := range refs[n] {
			if tr.Ref.Kind != refParam {
				continue
			}
			spec, ok := w.Parameters[tr.Ref.Name]
			if !ok {
				return &Error{Task: n, Field: tr.Field, Msg: fmt.Sprintf("references undeclared parameter %q", tr.Ref.Name)}
			}
			if !tr.Ref.Whole && (spec.Type == v2.PARAMETER_TYPE_LIST || spec.Type == v2.PARAMETER_TYPE_DICT) {
				return &Error{Task: n, Field: tr.Field, Msg: fmt.Sprintf("embeds parameter %q of type %s; only string, integer, number, and boolean parameters can be embedded in a longer string", tr.Ref.Name, typeName(spec.Type))}
			}
		}
	}
	return nil
}

// checkTaskRefs checks that each task output reference names another task
// that is a dependency and that has succeeded whenever the referencing task runs.
func checkTaskRefs(w *v2.DeclarativeWorkflow, g *graph, refs map[string][]taskRef) error {
	for _, n := range g.names {
		var deps, okDeps map[string]bool
		for _, tr := range refs[n] {
			if tr.Ref.Kind != refTaskOutput {
				continue
			}
			target := tr.Ref.Name
			if target == n {
				return &Error{Task: n, Field: tr.Field, Msg: "references the task itself"}
			}
			if _, ok := w.Tasks[target]; !ok {
				return &Error{Task: n, Field: tr.Field, Msg: fmt.Sprintf("references unknown task %q", target)}
			}
			if deps == nil {
				deps = g.transitiveDeps(n)
				okDeps = g.succeededDeps(n)
			}
			if !deps[target] {
				return &Error{Task: n, Field: tr.Field, Msg: fmt.Sprintf("references task %q that is not a dependency of %q", target, n)}
			}
			if !okDeps[target] {
				return &Error{Task: n, Field: tr.Field, Msg: fmt.Sprintf("references task %q, which may not have succeeded when %q runs; add depends_on {task: %s, when: SUCCEEDED} to %q or to a task it depends on", target, n, target, n)}
			}
		}
	}
	return nil
}
