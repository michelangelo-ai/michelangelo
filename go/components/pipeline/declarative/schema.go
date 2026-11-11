package declarative

import (
	"fmt"
	"sort"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ValidateAgainst runs the checks that need one release's task-type schemas.
// Call Validate first; this function assumes a well-formed graph.
func ValidateAgainst(w *v2.DeclarativeWorkflow, rel *Release) error {
	if rel == nil {
		return &Error{Msg: "release is not set"}
	}
	names := make([]string, 0, len(w.Tasks))
	for n := range w.Tasks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t := w.Tasks[n]
		tt, ok := rel.TaskTypes[t.TaskFunction]
		if !ok {
			return &Error{Task: n, Release: rel.Version, Msg: fmt.Sprintf("task_function %q is not a task type", t.TaskFunction)}
		}
		if r := effectiveRetry(w, t); r != nil && r.MaxAttempts > 1 && !tt.HasCapability("retry") {
			return &Error{Task: n, Release: rel.Version, Msg: fmt.Sprintf("retry is not a capability of task type %q", t.TaskFunction)}
		}
		sc := &sectionChecker{w: w, task: n, typeName: t.TaskFunction, release: rel.Version}
		if err := sc.check("config", t.Config, tt.Config); err != nil {
			return err
		}
		if err := sc.check("job_specs", t.JobSpecs, tt.JobSpecs); err != nil {
			return err
		}
	}
	return nil
}

func effectiveRetry(w *v2.DeclarativeWorkflow, t *v2.DeclarativeTask) *v2.RetryPolicy {
	if t.Retry != nil {
		return t.Retry
	}
	if w.WorkflowConfig != nil {
		return w.WorkflowConfig.DefaultRetry
	}
	return nil
}

type sectionChecker struct {
	w        *v2.DeclarativeWorkflow
	task     string
	typeName string
	release  string
}

func (c *sectionChecker) errf(field, format string, args ...interface{}) error {
	return &Error{Task: c.task, Field: field, Release: c.release, Msg: fmt.Sprintf(format, args...)}
}

// check validates one struct against one schema. prefix is "config" or
// "job_specs" at the top and grows with each group.
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
		if !ok {
			if f.Required {
				return c.errf(prefix, "field %q is required by task type %q", k, c.typeName)
			}
			continue
		}
		path := prefix + "." + k
		if f.Type == "" {
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

func (c *sectionChecker) leaf(path string, v *types.Value, slot string) error {
	if v == nil || v.Kind == nil {
		return c.errf(path, "must not be null")
	}
	if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
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
		if err := checkIntegral("", "value", nv.NumberValue); err != nil {
			return c.errf(path, "%s", err.(*Error).Msg)
		}
	case "number":
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return c.errf(path, "must be a number")
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
	}
	return nil
}

// reference checks a string value that holds references against the slot.
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
