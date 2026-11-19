package declarative

import (
	"fmt"
	"strings"
	"testing"

	gogoproto "github.com/gogo/protobuf/proto"
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
		}), Limits{}, `tasks form a depends_on cycle: a -> c -> b -> a`},
		{"cycle with downstream tasks", newWorkflow(map[string]*v2.DeclarativeTask{
			"a": task("core/python", "b"), "b": task("core/python", "a"), "c": task("core/python", "a"), "d": task("core/python", "c"),
		}), Limits{}, `tasks form a depends_on cycle: a -> b -> a`},
		{"unknown when", func() *v2.DeclarativeWorkflow {
			b := task("core/python")
			b.DependsOn = []*v2.TaskDependency{{Task: "a", When: 99}}
			return newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python"), "b": b})
		}(), Limits{}, `task "b": depends_on[0].when 99 is not a known outcome; use SUCCEEDED, FAILED, COMPLETED, or leave it unset`},
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
		want string
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

func TestValidateTaskOutputReferences(t *testing.T) {
	w := diamond()
	w.Tasks["publish"].Config.Fields["model"] = strVal("{{ tasks.train.output.model_uri }}")
	w.Tasks["publish"].Config.Fields["raw"] = strVal("{{ tasks.extract.output }}")
	require.NoError(t, Validate(w, Limits{}), "direct and transitive dependencies are both allowed")

	w.Tasks["evaluate"].Config.Fields["model"] = strVal("{{ tasks.train.output }}")
	assert.EqualError(t, Validate(w, Limits{}), `task "evaluate": config.model references task "train" that is not a dependency of "evaluate"`)

	w.Tasks["evaluate"].Config.Fields["model"] = strVal("{{ tasks.evaluate.output }}")
	assert.EqualError(t, Validate(w, Limits{}), `task "evaluate": config.model references the task itself`)

	w.Tasks["evaluate"].Config.Fields["model"] = strVal("{{ tasks.ghost.output }}")
	assert.EqualError(t, Validate(w, Limits{}), `task "evaluate": config.model references unknown task "ghost"`)
}

func FuzzValidate(f *testing.F) {
	seeds := []*v2.DeclarativeWorkflow{diamond(), altShape(), diamondFailure(),
		newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python", "zz")}),
		withParams(diamond(), map[string]*v2.ParameterSpec{"p": paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(1.5))}),
	}
	for _, s := range seeds {
		b, err := gogoproto.Marshal(s)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	rels, err := LoadReleases("testdata/releases")
	if err != nil {
		f.Fatal(err)
	}
	rel := rels["core-v1.4.2"]
	f.Fuzz(func(t *testing.T, b []byte) {
		w := &v2.DeclarativeWorkflow{}
		if err := gogoproto.Unmarshal(b, w); err != nil {
			return
		}
		_ = Validate(w, Limits{})
		_ = ValidateAgainst(w, Limits{}, rel)
		_, _ = Plan(w, Limits{}, rel, nil)
		if g, err := buildGraph(w); err == nil {
			_ = g.levels()
		}
	})
}

func TestValidateOutputRefNeedsSucceededLastEdge(t *testing.T) {
	const want = `task "notify": config.model references task "train", which may not have succeeded when "notify" runs; ` +
		`add depends_on {task: train, when: SUCCEEDED} to "notify" or to a task it depends on`
	notifyOn := func(w *v2.DeclarativeWorkflow, dep string, when v2.TaskDependency_Outcome) *v2.DeclarativeWorkflow {
		notify := task("core/python")
		notify.DependsOn = []*v2.TaskDependency{{Task: dep, When: when}}
		notify.Config.Fields["model"] = strVal("{{ tasks.train.output.model_uri }}")
		w.Tasks["notify"] = notify
		return w
	}
	for _, when := range []v2.TaskDependency_Outcome{v2.OUTCOME_UNSPECIFIED, v2.OUTCOME_SUCCEEDED, v2.OUTCOME_FAILED, v2.OUTCOME_COMPLETED} {
		assert.NoError(t, Validate(notifyOn(diamond(), "publish", when), Limits{}), "publish ran, so train succeeded: when=%v", when)
	}
	assert.EqualError(t, Validate(notifyOn(diamond(), "train", v2.OUTCOME_FAILED), Limits{}), want)
	assert.EqualError(t, Validate(notifyOn(diamond(), "train", v2.OUTCOME_COMPLETED), Limits{}), want)

	twoHops := diamond()
	twoHops.Tasks["publish"].DependsOn = []*v2.TaskDependency{{Task: "train", When: v2.OUTCOME_COMPLETED}, {Task: "evaluate"}}
	assert.EqualError(t, Validate(notifyOn(twoHops, "publish", v2.OUTCOME_SUCCEEDED), Limits{}), want)

	bothPaths := diamond()
	bothPaths.Tasks["publish"].DependsOn = []*v2.TaskDependency{{Task: "train", When: v2.OUTCOME_COMPLETED}, {Task: "evaluate"}}
	bothPaths.Tasks["evaluate"].DependsOn = append(bothPaths.Tasks["evaluate"].DependsOn, &v2.TaskDependency{Task: "train"})
	assert.NoError(t, Validate(notifyOn(bothPaths, "publish", v2.OUTCOME_SUCCEEDED), Limits{}), "evaluate ran, so train succeeded")
}

func TestValidateRejectsTooManyDependencies(t *testing.T) {
	w := newWorkflow(map[string]*v2.DeclarativeTask{
		"a": task("core/python"),
		"b": task("core/python", "a"),
		"c": task("core/python", "a", "b"),
	})
	assert.EqualError(t, Validate(w, Limits{MaxDependencies: 2}), "tasks have 3 depends_on entries in total; the limit is 2")
	assert.NoError(t, Validate(w, Limits{MaxDependencies: 3}))
}

// denseWorkflow is the costliest shape that fits the default limits: a fully
// connected block of 96 tasks, then a chain, where each task reads the output
// of its last dependency.
func denseWorkflow() *v2.DeclarativeWorkflow {
	name := func(i int) string { return fmt.Sprintf("t%03d", i) }
	ts := map[string]*v2.DeclarativeTask{}
	for i := 0; i < DefaultLimits().MaxTasks; i++ {
		var deps []string
		if i < 96 {
			for j := 0; j < i; j++ {
				deps = append(deps, name(j))
			}
		} else {
			deps = []string{name(i - 1)}
		}
		tk := task("core/python", deps...)
		if len(deps) > 0 {
			tk.Config.Fields["uri"] = strVal("{{ tasks." + deps[len(deps)-1] + ".output }}")
		}
		ts[name(i)] = tk
	}
	return typed(newWorkflow(ts))
}

func BenchmarkValidateDense(b *testing.B) {
	w := denseWorkflow()
	require.NoError(b, Validate(w, Limits{}))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Validate(w, Limits{})
	}
}
