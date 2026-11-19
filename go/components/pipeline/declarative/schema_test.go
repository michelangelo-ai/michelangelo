package declarative

import (
	"math"
	"testing"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadRelease(t *testing.T) *Release {
	t.Helper()
	rs, err := LoadReleases("testdata/releases")
	require.NoError(t, err)
	return rs["core-v1.4.2"]
}

func typedDiamond() *v2.DeclarativeWorkflow { return typed(diamond()) }

func TestValidateAgainstAccepts(t *testing.T) {
	w := typedDiamond()
	w.Tasks["train"].Config.Fields["epochs"] = numVal(3)
	w.Tasks["train"].Config.Fields["learning_rate"] = numVal(0.1)
	w.Tasks["train"].Config.Fields["debug"] = boolVal(false)
	w.Tasks["train"].Config.Fields["args"] = listVal(strVal("--fast"))
	w.Tasks["train"].Config.Fields["extra"] = structVal(map[string]*types.Value{"k": numVal(1)})
	w.Tasks["train"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{
		"ray": structVal(map[string]*types.Value{"head_cpu": numVal(4), "image": strVal("img")}),
	}}
	w.Tasks["train"].Retry = &v2.RetryPolicy{MaxAttempts: 3}
	require.NoError(t, Validate(w, Limits{}))
	require.NoError(t, ValidateAgainst(w, Limits{}, loadRelease(t)))
}

func TestValidateAgainstErrors(t *testing.T) {
	rel := loadRelease(t)
	cases := []struct {
		name string
		edit func(w *v2.DeclarativeWorkflow)
		want string
	}{
		{"unknown task_function", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].TaskFunction = "core/spark" },
			`task "train": task_function "core/spark" is not a task type in core-v1.4.2`},
		{"retry without capability", func(w *v2.DeclarativeWorkflow) { w.Tasks["extract"].Retry = &v2.RetryPolicy{MaxAttempts: 2} },
			`task "extract": retry is not a capability of task type "core/python" in core-v1.4.2`},
		{"inherited retry without capability", func(w *v2.DeclarativeWorkflow) {
			w.WorkflowConfig = &v2.WorkflowSettings{DefaultRetry: &v2.RetryPolicy{MaxAttempts: 2}}
		}, `task "evaluate": retry is not a capability of task type "core/python" in core-v1.4.2`},
		{"single attempt needs no capability", func(w *v2.DeclarativeWorkflow) { w.Tasks["extract"].Retry = &v2.RetryPolicy{MaxAttempts: 1} }, ""},
		{"undeclared config field", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["regions"] = strVal("x") },
			`task "train": config field "regions" is not declared by task type "core/ray" in core-v1.4.2`},
		{"undeclared nested job_specs field", func(w *v2.DeclarativeWorkflow) {
			w.Tasks["train"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{"ray": structVal(map[string]*types.Value{"regions": strVal("x")})}}
		}, `task "train": job_specs.ray field "regions" is not declared by task type "core/ray" in core-v1.4.2`},
		{"missing required", func(w *v2.DeclarativeWorkflow) { delete(w.Tasks["train"].Config.Fields, "entry_point") },
			`task "train": config field "entry_point" is required by task type "core/ray" in core-v1.4.2`},
		{"nil config with required", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config = nil },
			`task "train": config field "entry_point" is required by task type "core/ray" in core-v1.4.2`},
		{"group given scalar", func(w *v2.DeclarativeWorkflow) {
			w.Tasks["train"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{"ray": numVal(1)}}
		}, `task "train": job_specs.ray must be an object in core-v1.4.2`},
		{"string slot gets number", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["entry_point"] = numVal(1) },
			`task "train": config.entry_point must be a string in core-v1.4.2`},
		{"integer slot gets fraction", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["epochs"] = numVal(1.5) },
			`task "train": config.epochs value 1.5 is not an integer in core-v1.4.2`},
		{"integer slot gets string", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["epochs"] = strVal("3") },
			`task "train": config.epochs must be a number in core-v1.4.2`},
		{"number slot gets bool", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["learning_rate"] = boolVal(true) },
			`task "train": config.learning_rate must be a number in core-v1.4.2`},
		{"boolean slot gets string", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["debug"] = strVal("true") },
			`task "train": config.debug must be a bool in core-v1.4.2`},
		{"list slot gets string", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["args"] = strVal("--fast") },
			`task "train": config.args must be a list in core-v1.4.2`},
		{"dict slot gets list", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["extra"] = listVal() },
			`task "train": config.extra must be an object in core-v1.4.2`},
		{"null value", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["epochs"] = nullVal() },
			`task "train": config.epochs must not be null in core-v1.4.2`},
		{"param ref matching type", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["n"] = paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(2))
			w.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.n }}")
		}, ""},
		{"integer widens into number", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["n"] = paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(2))
			w.Tasks["train"].Config.Fields["learning_rate"] = strVal("{{ params.n }}")
		}, ""},
		{"number does not narrow into integer", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["lr"] = paramSpec(v2.PARAMETER_TYPE_NUMBER, numVal(0.5))
			w.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.lr }}")
		}, `task "train": config.epochs references parameter "lr" of type number; the slot type is integer in core-v1.4.2`},
		{"string param into integer slot", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["s"] = paramSpec(v2.PARAMETER_TYPE_STRING, strVal("3"))
			w.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.s }}")
		}, `task "train": config.epochs references parameter "s" of type string; the slot type is integer in core-v1.4.2`},
		{"embedded ref in integer slot", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["n"] = paramSpec(v2.PARAMETER_TYPE_INTEGER, numVal(2))
			w.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.n }}0")
		}, `task "train": config.epochs embeds a reference; the slot type is integer, not string in core-v1.4.2`},
		{"task output ref in any slot", func(w *v2.DeclarativeWorkflow) {
			w.Tasks["publish"].Config.Fields["model"] = strVal("{{ tasks.train.output.model_uri }}")
			w.Tasks["train"].Config.Fields["epochs"] = strVal("{{ tasks.extract.output.n }}")
		}, ""},
		{"NaN in number slot", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["learning_rate"] = numVal(math.NaN()) },
			`task "train": config.learning_rate must be finite in core-v1.4.2`},
		{"+Inf in number slot", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["learning_rate"] = numVal(math.Inf(1)) },
			`task "train": config.learning_rate must be finite in core-v1.4.2`},
		{"-Inf in number slot", func(w *v2.DeclarativeWorkflow) {
			w.Tasks["train"].Config.Fields["learning_rate"] = numVal(math.Inf(-1))
		},
			`task "train": config.learning_rate must be finite in core-v1.4.2`},
		{"finite number slot", func(w *v2.DeclarativeWorkflow) { w.Tasks["train"].Config.Fields["learning_rate"] = numVal(-0.5) }, ""},
		{"list element ref", func(w *v2.DeclarativeWorkflow) {
			w.Parameters["xs"] = paramSpec(v2.PARAMETER_TYPE_LIST, listVal())
			w.Tasks["train"].Config.Fields["args"] = listVal(strVal("{{ params.xs }}"))
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := typedDiamond()
			c.edit(w)
			require.NoError(t, Validate(w, Limits{}), "fixture must pass Validate first")
			err := ValidateAgainst(w, Limits{}, rel)
			if c.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, c.want)
		})
	}
}

func TestValidateAgainstNilRelease(t *testing.T) {
	assert.EqualError(t, ValidateAgainst(typedDiamond(), Limits{}, nil), "release is not set")
}

func TestValidateAgainstRejectsUnvalidatedInput(t *testing.T) {
	rel := loadRelease(t)
	undeclared := typedDiamond()
	undeclared.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.nope }}")
	nullGroup := typedDiamond()
	nullGroup.Tasks["train"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{"ray": nil}}
	jsonNullGroup := typedDiamond()
	jsonNullGroup.Tasks["train"].JobSpecs = &types.Struct{Fields: map[string]*types.Value{"ray": nullVal()}}
	nilFieldRel := &Release{Version: "core-v0", TaskTypes: map[string]*TaskType{"core/python": {Config: Schema{"entry_point": nil}}}}
	nilTypeRel := &Release{Version: "core-v0", TaskTypes: map[string]*TaskType{"core/python": nil, "core/ray": nil}}
	floatRel := &Release{Version: "core-v0", TaskTypes: map[string]*TaskType{"core/python": {Config: Schema{
		"entry_point": {Type: "string", Required: true},
		"n":           {Type: "float"},
	}}}}
	floatSlot := typedDiamond()
	floatSlot.Tasks["evaluate"].Config.Fields["n"] = boolVal(true)
	cases := []struct {
		name string
		w    *v2.DeclarativeWorkflow
		rel  *Release
		want string
	}{
		{"nil workflow", nil, rel, "workflow is not set"},
		{"nil task", newWorkflow(map[string]*v2.DeclarativeTask{"a": nil}), rel, `task "a": task is not set`},
		{"undeclared parameter", undeclared, rel, `task "train": config.epochs references undeclared parameter "nope"`},
		{"null group value", nullGroup, rel, `task "train": job_specs.ray must not be null in core-v1.4.2`},
		{"json null group value", jsonNullGroup, rel, `task "train": job_specs.ray must not be null in core-v1.4.2`},
		{"nil schema field", typedDiamond(), nilFieldRel, `task "evaluate": config.entry_point has no schema in task type "core/python" in core-v0`},
		{"unknown slot type", floatSlot, floatRel, `task "evaluate": config.n has unknown slot type "float" in task type "core/python" in core-v0`},
		{"nil task type", typedDiamond(), nilTypeRel, `task "evaluate": task_function "core/python" is not a task type in core-v0`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.EqualError(t, ValidateAgainst(c.w, Limits{}, c.rel), c.want)
		})
	}
}

func TestValidateAgainstRequiredInsideAbsentGroup(t *testing.T) {
	rel, err := ParseRelease("core-v2.yaml", []byte(`version: core-v2
artifact: s3://x
digest: sha256:ab
task_types:
  core/python:
    config:
      entry_point: {type: string, required: true}
      res:
        cpu: {type: integer, required: true}
        mem: {type: integer}
`))
	require.NoError(t, err)
	w := newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python")})
	w.Tasks["a"].Config.Fields["entry_point"] = strVal("main.py")
	assert.EqualError(t, ValidateAgainst(w, Limits{}, rel), `task "a": config.res field "cpu" is required by task type "core/python" in core-v2`)
	w.Tasks["a"].Config.Fields["res"] = structVal(map[string]*types.Value{"cpu": numVal(2)})
	assert.NoError(t, ValidateAgainst(w, Limits{}, rel))
}
