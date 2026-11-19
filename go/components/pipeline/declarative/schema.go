package declarative

import (
	"fmt"
	"math"
	"sort"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ValidateAgainst runs Validate, then checks w against the task-type schemas
// of rel.
func ValidateAgainst(w *v2.DeclarativeWorkflow, lim Limits, rel *Release) error {
	_, err := validateAgainst(w, lim, rel)
	return err
}

// validateAgainst runs ValidateAgainst and returns the depends_on graph that
// Validate built.
func validateAgainst(w *v2.DeclarativeWorkflow, lim Limits, rel *Release) (*graph, error) {
	if rel == nil {
		return nil, &Error{Msg: "release is not set"}
	}
	g, err := validate(w, lim)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(w.Tasks))
	for n := range w.Tasks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := w.Tasks[n]
		tt := rel.TaskTypes[t.TaskFunction]
		if tt == nil {
			return nil, &Error{Task: n, Release: rel.Version, Msg: fmt.Sprintf("task_function %q is not a task type", t.TaskFunction)}
		}
		if r := effectiveRetry(w, t); r != nil && r.MaxAttempts > 1 && !tt.HasCapability("retry") {
			return nil, &Error{Task: n, Release: rel.Version, Msg: fmt.Sprintf("retry is not a capability of task type %q", t.TaskFunction)}
		}
		sc := &sectionChecker{w: w, task: n, typeName: t.TaskFunction, release: rel.Version}
		if err := sc.check("config", t.Config, tt.Config); err != nil {
			return nil, err
		}
		if err := sc.check("job_specs", t.JobSpecs, tt.JobSpecs); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// effectiveRetry returns the retry policy of t, or the workflow default when
// t sets none.
func effectiveRetry(w *v2.DeclarativeWorkflow, t *v2.DeclarativeTask) *v2.RetryPolicy {
	if t.Retry != nil {
		return t.Retry
	}
	if w.WorkflowConfig != nil {
		return w.WorkflowConfig.DefaultRetry
	}
	return nil
}

// sectionChecker checks the config or job_specs of one task against the
// schema of its task type.
type sectionChecker struct {
	w        *v2.DeclarativeWorkflow
	task     string
	typeName string
	release  string
}

// errf returns an Error for field, tagged with the task and the release.
func (c *sectionChecker) errf(field, format string, args ...any) error {
	return &Error{Task: c.task, Field: field, Release: c.release, Msg: fmt.Sprintf(format, args...)}
}

// check checks s against schema: unknown keys, missing required fields, and
// each value against its slot type. A group that is absent is checked as
// empty, so its required fields are reported as missing.
func (c *sectionChecker) check(prefix string, s *types.Struct, schema Schema) error {
	var fields map[string]*types.Value
	if s != nil {
		fields = s.Fields
	}
	present := make([]string, 0, len(fields))
	for k := range fields {
		present = append(present, k)
	}
	sort.Strings(present)
	for _, k := range present {
		if _, ok := schema[k]; !ok {
			return c.errf(prefix, "field %q is not declared by task type %q", k, c.typeName)
		}
	}
	declared := make([]string, 0, len(schema))
	for k := range schema {
		declared = append(declared, k)
	}
	sort.Strings(declared)
	for _, k := range declared {
		f := schema[k]
		v, ok := fields[k]
		path := prefix + "." + k
		if f == nil {
			return c.errf(path, "has no schema in task type %q", c.typeName)
		}
		if !ok {
			if f.Required {
				return c.errf(prefix, "field %q is required by task type %q", k, c.typeName)
			}
			if f.Type == "" {
				if err := c.check(path, nil, f.Fields); err != nil {
					return err
				}
			}
			continue
		}
		if f.Type == "" {
			if isNull(v) {
				return c.errf(path, "must not be null")
			}
			sv, ok := v.Kind.(*types.Value_StructValue)
			if !ok {
				return c.errf(path, "must be an object")
			}
			if err := c.check(path, sv.StructValue, f.Fields); err != nil {
				return err
			}
			continue
		}
		if err := c.leaf(path, v, f.Type); err != nil {
			return err
		}
	}
	return nil
}

// leaf checks one value against its slot type. A string that holds
// references is checked by reference instead.
func (c *sectionChecker) leaf(path string, v *types.Value, slot string) error {
	if isNull(v) {
		return c.errf(path, "must not be null")
	}
	if sv, ok := v.Kind.(*types.Value_StringValue); ok {
		refs, err := parseRefs(sv.StringValue)
		if err != nil {
			return c.errf(path, "%s", err.Error())
		}
		if len(refs) > 0 {
			return c.reference(path, refs, slot)
		}
	}
	switch slot {
	case "string":
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return c.errf(path, "must be a string")
		}
	case "integer":
		nv, ok := v.Kind.(*types.Value_NumberValue)
		if !ok {
			return c.errf(path, "must be a number")
		}
		if msg := integralProblem("value", nv.NumberValue); msg != "" {
			return c.errf(path, "%s", msg)
		}
	case "number":
		nv, ok := v.Kind.(*types.Value_NumberValue)
		if !ok {
			return c.errf(path, "must be a number")
		}
		if math.IsNaN(nv.NumberValue) || math.IsInf(nv.NumberValue, 0) {
			return c.errf(path, "must be finite")
		}
	case "boolean":
		if _, ok := v.Kind.(*types.Value_BoolValue); !ok {
			return c.errf(path, "must be a bool")
		}
	case "list":
		if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return c.errf(path, "must be a list")
		}
	case "dict":
		if _, ok := v.Kind.(*types.Value_StructValue); !ok {
			return c.errf(path, "must be an object")
		}
	default:
		return c.errf(path, "has unknown slot type %q in task type %q", slot, c.typeName)
	}
	return nil
}

// reference checks that a string with references fits its slot. A reference
// embedded in a longer string fits only a string slot. A whole parameter
// reference must have the slot type, or integer for a number slot. A whole task
// output reference fits any slot, because the output type is known only at run time.
func (c *sectionChecker) reference(path string, refs []ref, slot string) error {
	whole := len(refs) == 1 && refs[0].Whole
	if !whole {
		if slot != "string" {
			return c.errf(path, "embeds a reference; the slot type is %s, not string", slot)
		}
		return nil
	}
	r := refs[0]
	if r.Kind == refTaskOutput {
		return nil
	}
	spec := c.w.Parameters[r.Name]
	pt := typeName(spec.Type)
	if pt == slot || (pt == "integer" && slot == "number") {
		return nil
	}
	return c.errf(path, "references parameter %q of type %s; the slot type is %s", r.Name, pt, slot)
}

// isNull reports whether v is missing or holds a JSON null.
func isNull(v *types.Value) bool {
	if v == nil || v.Kind == nil {
		return true
	}
	_, null := v.Kind.(*types.Value_NullValue)
	return null
}
