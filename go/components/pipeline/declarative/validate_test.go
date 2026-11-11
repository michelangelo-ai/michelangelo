package declarative

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsDiamond(t *testing.T) {
	require.NoError(t, Validate(diamond(), Limits{}))
}

func TestValidateStructural(t *testing.T) {
	cases := []struct {
		name string
		w    *v2.DeclarativeWorkflow
		lim  Limits
		want string
	}{
		{"schema version", func() *v2.DeclarativeWorkflow { w := diamond(); w.SchemaVersion = "v1"; return w }(), Limits{},
			`schema_version "v1" is not supported; use "v1alpha1"`},
		{"empty graph", newWorkflow(map[string]*v2.DeclarativeTask{}), Limits{},
			"tasks must have at least one entry"},
		{"nil tasks", &v2.DeclarativeWorkflow{SchemaVersion: SchemaVersion}, Limits{},
			"tasks must have at least one entry"},
		{"bad name uppercase", newWorkflow(map[string]*v2.DeclarativeTask{"Train": task("core/ray")}), Limits{},
			`task "Train": name must match ^[a-z][a-z0-9_]{0,62}$`},
		{"bad name leading digit", newWorkflow(map[string]*v2.DeclarativeTask{"1a": task("core/ray")}), Limits{},
			`task "1a": name must match ^[a-z][a-z0-9_]{0,62}$`},
		{"name too long", newWorkflow(map[string]*v2.DeclarativeTask{"a" + strings.Repeat("b", 63): task("core/ray")}), Limits{},
			`task "` + "a" + strings.Repeat("b", 63) + `": name must match ^[a-z][a-z0-9_]{0,62}$`},
		{"too many tasks", func() *v2.DeclarativeWorkflow {
			ts := map[string]*v2.DeclarativeTask{}
			for i := 0; i < 3; i++ {
				ts[fmt.Sprintf("t%d", i)] = task("core/python")
			}
			return newWorkflow(ts)
		}(), Limits{MaxTasks: 2}, "tasks has 3 entries; the limit is 2"},
		{"nil task", newWorkflow(map[string]*v2.DeclarativeTask{"a": nil}), Limits{},
			`task "a": task is not set`},
		{"empty task_function", newWorkflow(map[string]*v2.DeclarativeTask{"a": task("")}), Limits{},
			`task "a": task_function is not set`},
		{"unknown dependency", newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python", "zz")}), Limits{},
			`task "a": depends_on names unknown task "zz"`},
		{"self dependency", newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python", "a")}), Limits{},
			`task "a": depends_on names the task itself`},
		{"duplicate edge", func() *v2.DeclarativeWorkflow {
			b := task("core/python", "a")
			b.DependsOn = append(b.DependsOn, &v2.TaskDependency{Task: "a", When: v2.OUTCOME_FAILED})
			return newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python"), "b": b})
		}(), Limits{}, `task "b": depends_on names "a" more than once`},
		{"empty dependency name", func() *v2.DeclarativeWorkflow {
			b := task("core/python")
			b.DependsOn = []*v2.TaskDependency{{Task: ""}}
			return newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python"), "b": b})
		}(), Limits{}, `task "b": depends_on[0].task is not set`},
		{"cycle", newWorkflow(map[string]*v2.DeclarativeTask{
			"a": task("core/python", "c"), "b": task("core/python", "a"), "c": task("core/python", "b"), "d": task("core/python"),
		}), Limits{}, `tasks form a cycle: a, b, c`},
		{"unknown dependency before cycle", newWorkflow(map[string]*v2.DeclarativeTask{
			"a": task("core/python", "b"), "b": task("core/python", "a", "zz"),
		}), Limits{}, `task "b": depends_on names unknown task "zz"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.w, c.lim)
			require.Error(t, err)
			assert.EqualError(t, err, c.want)
			var e *Error
			assert.ErrorAs(t, err, &e)
		})
	}
}

func withParams(w *v2.DeclarativeWorkflow, ps map[string]*v2.ParameterSpec) *v2.DeclarativeWorkflow {
	w.Parameters = ps
	return w
}

func TestValidateParameters(t *testing.T) {
	big := float64(1<<53 - 1)
	cases := []struct {
		name string
		spec *v2.ParameterSpec
		want string // empty means accepted
	}{
		{"string ok", paramSpec(v2.PARAMETER_TYPE_STRING, strVal("x")), ""},
		{"integer ok", paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(3)), ""},
		{"integer max ok", paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(big)), ""},
		{"integer negative ok", paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(-7)), ""},
		{"number ok", paramSpec(v2.PARAMETER_TYPE_NUMBER, numVal(0.5)), ""},
		{"number integral ok", paramSpec(v2.PARAMETER_TYPE_NUMBER, numVal(2)), ""},
		{"boolean ok", paramSpec(v2.PARAMETER_TYPE_BOOLEAN, boolVal(true)), ""},
		{"list ok", paramSpec(v2.PARAMETER_TYPE_LIST, listVal(numVal(1), strVal("a"))), ""},
		{"dict ok", paramSpec(v2.PARAMETER_TYPE_DICT, structVal(map[string]*types.Value{"k": numVal(1)})), ""},
		{"required ok", &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_STRING, Required: true}, ""},
		{"unspecified type", paramSpec(v2.PARAMETER_TYPE_UNSPECIFIED, strVal("x")), "type is not set"},
		{"unknown type value", paramSpec(v2.ParameterType(99), strVal("x")), "type 99 is not a known ParameterType"},
		{"neither default nor required", &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_STRING}, "exactly one of default or required must be set"},
		{"both default and required", &v2.ParameterSpec{Type: v2.PARAMETER_TYPE_STRING, Required: true, Default: strVal("x")}, "exactly one of default or required must be set"},
		{"null default", paramSpec(v2.PARAMETER_TYPE_STRING, nullVal()), "default must not be null"},
		{"empty value", paramSpec(v2.PARAMETER_TYPE_STRING, &types.Value{}), "default must not be null"},
		{"string gets number", paramSpec(v2.PARAMETER_TYPE_STRING, numVal(1)), "default must be a string for type string"},
		{"integer gets fraction", paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(1.5)), "default 1.5 is not an integer"},
		{"integer too large", paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(big*2)), "default 18014398509481982 is outside ±9007199254740991"},
		{"integer gets string", paramSpec(v2.PARAMETER_TYPE_INTEGER, strVal("1")), "default must be a number for type integer"},
		{"number gets bool", paramSpec(v2.PARAMETER_TYPE_NUMBER, boolVal(true)), "default must be a number for type number"},
		{"boolean gets string", paramSpec(v2.PARAMETER_TYPE_BOOLEAN, strVal("true")), "default must be a bool for type boolean"},
		{"list gets struct", paramSpec(v2.PARAMETER_TYPE_LIST, structVal(nil)), "default must be a list for type list"},
		{"dict gets list", paramSpec(v2.PARAMETER_TYPE_DICT, listVal()), "default must be a struct for type dict"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := withParams(diamond(), map[string]*v2.ParameterSpec{"p": c.spec})
			err := Validate(w, Limits{})
			if c.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, `parameter "p": `+c.want)
		})
	}
}

func TestValidateParameterName(t *testing.T) {
	w := withParams(diamond(), map[string]*v2.ParameterSpec{"Bad": paramSpec(v2.PARAMETER_TYPE_STRING, strVal("x"))})
	assert.EqualError(t, Validate(w, Limits{}), `parameter "Bad": name must match ^[a-z][a-z0-9_]{0,62}$`)
}

func TestValidateNilParameter(t *testing.T) {
	w := withParams(diamond(), map[string]*v2.ParameterSpec{"p": nil})
	assert.EqualError(t, Validate(w, Limits{}), `parameter "p": parameter is not set`)
}

func TestValidateParamReferences(t *testing.T) {
	w := withParams(diamond(), map[string]*v2.ParameterSpec{"bucket": paramSpec(v2.PARAMETER_TYPE_STRING, strVal("b"))})
	w.Tasks["extract"].Config.Fields["uri"] = strVal("gs://{{ params.bucket }}/in")
	require.NoError(t, Validate(w, Limits{}))

	w.Tasks["extract"].Config.Fields["uri"] = strVal("gs://{{ params.missing }}/in")
	assert.EqualError(t, Validate(w, Limits{}), `task "extract": config.uri references undeclared parameter "missing"`)

	w.Tasks["extract"].Config.Fields["uri"] = strVal("gs://{{ params.bucket")
	assert.EqualError(t, Validate(w, Limits{}), `task "extract": config.uri unterminated "{{" in "gs://{{ params.bucket"`)

	w.Tasks["extract"].Config.Fields["uri"] = strVal("ok")
	w.Tasks["extract"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{
		"ray": structVal(map[string]*types.Value{"image": strVal("{{ params.nope }}")}),
	}}
	assert.EqualError(t, Validate(w, Limits{}), `task "extract": job_specs.ray.image references undeclared parameter "nope"`)

	w.Tasks["extract"].JobSpecs = nil
	w.Tasks["extract"].Config.Fields["args"] = listVal(strVal("--in={{ params.gone }}"))
	assert.EqualError(t, Validate(w, Limits{}), `task "extract": config.args[0] references undeclared parameter "gone"`)
}

func TestValidateEmbeddedListParam(t *testing.T) {
	w := withParams(diamond(), map[string]*v2.ParameterSpec{"xs": paramSpec(v2.PARAMETER_TYPE_LIST, listVal())})
	w.Tasks["extract"].Config.Fields["a"] = strVal("{{ params.xs }}")
	require.NoError(t, Validate(w, Limits{}))
	w.Tasks["extract"].Config.Fields["a"] = strVal("x={{ params.xs }}")
	assert.EqualError(t, Validate(w, Limits{}), `task "extract": config.a embeds parameter "xs" of type list; only string, integer, number, and boolean parameters can be embedded in a longer string`)
}
