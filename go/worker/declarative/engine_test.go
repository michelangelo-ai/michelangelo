package declarative

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// edge is a depends_on entry with an explicit gate.
type edge struct{ task, when string }

// task returns a task as WorkflowArgs emits it. A string dependency has the
// OUTCOME_UNSPECIFIED gate.
func task(typ string, deps ...any) map[string]any {
	dependsOn := []any{}
	for _, d := range deps {
		switch d := d.(type) {
		case string:
			dependsOn = append(dependsOn, map[string]any{"task": d, "when": "OUTCOME_UNSPECIFIED"})
		case edge:
			dependsOn = append(dependsOn, map[string]any{"task": d.task, "when": d.when})
		}
	}
	return map[string]any{
		"task_function": typ,
		"depends_on":    dependsOn,
		"config":        map[string]any{"entry_point": "x.py"},
		"job_specs":     nil,
		"retry":         nil,
	}
}

func spec(workflowConfig map[string]any, tasks map[string]any) map[string]any {
	return map[string]any{
		"schema_version":  "v1alpha1",
		"workflow_config": workflowConfig,
		"parameters":      map[string]any{},
		"tasks":           tasks,
	}
}

// okRecord is what fake/ok returns for a task with no args, kwargs, fields,
// or retry policy.
func okRecord(alias string) map[string]any {
	return map[string]any{"ep": "x.py", "alias": alias, "args": []any{}, "kwargs": map[string]any{}, "retry": "absent", "fields": map[string]any{}}
}

type engineRun struct {
	result map[string]any
	// states is the run view: the last state each task reported.
	states map[string]string
	// messages holds the last non-empty message each task reported.
	messages map[string]string
	err      string
}

func runEngine(t *testing.T, tar []byte, fn string, args ...any) engineRun {
	t.Helper()
	res, progress, err := runStar(t, tar, "/cases.star", fn, args...)
	out := engineRun{states: map[string]string{}, messages: map[string]string{}}
	for _, line := range progress {
		var p struct {
			TaskName    string `json:"task_name"`
			TaskState   string `json:"task_state"`
			TaskMessage string `json:"task_message"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &p), line)
		out.states[p.TaskName] = p.TaskState
		if p.TaskMessage != "" {
			out.messages[p.TaskName] = p.TaskMessage
		}
	}
	if err != nil {
		out.err = err.Error()
		return out
	}
	out.result = toGo(t, res).(map[string]any)
	return out
}

func casesTar(t *testing.T) []byte {
	return dirTar(t, "testdata/root")
}

const (
	succeeded = "SUCCEEDED"
	failed    = "FAILED"
	skipped   = "SKIPPED"
)

func TestEngineGraph(t *testing.T) {
	tar := casesTar(t)
	for _, tc := range []struct {
		name    string
		spec    map[string]any
		states  map[string]string
		outputs map[string]any
		err     string
	}{
		{
			name: "test_diamond_happy",
			spec: spec(map[string]any{"max_concurrency": 4}, map[string]any{
				"root":  task("fake/ok"),
				"left":  task("fake/ok", "root"),
				"right": task("fake/ok", "root"),
				"join":  task("fake/ok", "left", "right"),
			}),
			states:  map[string]string{"root": succeeded, "left": succeeded, "right": succeeded, "join": succeeded},
			outputs: map[string]any{"root": okRecord("root"), "left": okRecord("left"), "right": okRecord("right"), "join": okRecord("join")},
		},
		{
			name: "test_failure_semantics",
			spec: spec(nil, map[string]any{
				"root":   task("fake/ok"),
				"left":   task("fake/fail", "root"),
				"right":  task("fake/ok", "root"),
				"join":   task("fake/ok", "left", "right"),
				"report": task("fake/ok", "join"),
			}),
			states: map[string]string{"root": succeeded, "left": failed, "right": succeeded, "join": skipped, "report": skipped},
			err:    "declarative: failed task(s): left",
		},
		{
			name: "test_gate_completed_and_failed_outcomes",
			spec: spec(nil, map[string]any{
				"a": task("fake/fail"),
				"b": task("fake/ok", edge{"a", "OUTCOME_COMPLETED"}),
				"c": task("fake/ok", edge{"a", "OUTCOME_SUCCEEDED"}),
				"d": task("fake/ok", edge{"a", "OUTCOME_FAILED"}),
			}),
			states: map[string]string{"a": failed, "b": succeeded, "c": skipped, "d": succeeded},
			err:    "declarative: failed task(s): a",
		},
		{
			name: "test_skip_only_run_succeeds",
			spec: spec(nil, map[string]any{
				"a": task("fake/ok"),
				"b": task("fake/ok", edge{"a", "OUTCOME_FAILED"}),
				"c": task("fake/ok", "b"),
			}),
			states:  map[string]string{"a": succeeded, "b": skipped, "c": skipped},
			outputs: map[string]any{"a": okRecord("a"), "b": nil, "c": nil},
		},
		{
			name: "test_validation_cycle",
			spec: spec(nil, map[string]any{
				"a": task("fake/ok", "b"),
				"b": task("fake/ok", "a"),
				"c": task("fake/ok"),
			}),
			states: map[string]string{},
			err:    "declarative: cycle among tasks: a, b",
		},
		{
			name:   "test_validation_self_edge",
			spec:   spec(nil, map[string]any{"a": task("fake/ok", "a")}),
			states: map[string]string{},
			err:    "declarative: cycle among tasks: a",
		},
		{
			name:   "test_validation_unknown_dep",
			spec:   spec(nil, map[string]any{"a": task("fake/ok", "ghost")}),
			states: map[string]string{},
			err:    `declarative: task "a" depends on unknown task "ghost"`,
		},
		{
			name:   "test_validation_unknown_function",
			spec:   spec(nil, map[string]any{"a": task("fake/ghost")}),
			states: map[string]string{},
			err:    `declarative: task "a" has type "fake/ghost", which is not in the registry`,
		},
		{
			name:   "validation_unknown_gate",
			spec:   spec(nil, map[string]any{"a": task("fake/ok"), "b": task("fake/ok", edge{"a", "FAILED"})}),
			states: map[string]string{},
			err:    `declarative: task "b" has unknown gate "FAILED" on "a"`,
		},
		{
			name:    "empty_tasks",
			spec:    spec(nil, map[string]any{}),
			states:  map[string]string{},
			outputs: map[string]any{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runEngine(t, tar, "run_case", tc.spec, map[string]any{})
			assert.Equal(t, tc.states, got.states, "run view")
			if tc.err != "" {
				assert.Equal(t, "fail: "+tc.err, got.err)
				return
			}
			require.Empty(t, got.err)
			assert.Equal(t, map[string]any{"states": toAny(tc.states), "outputs": tc.outputs}, got.result)
		})
	}
}

func toAny(states map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range states {
		out[k] = v
	}
	return out
}

// startGroups groups the tasks of a fake/sleep run by start time, in time
// order, so a test can see which tasks ran side by side.
func startGroups(t *testing.T, outputs map[string]any) [][]string {
	t.Helper()
	byStart := map[int][]string{}
	for name, out := range outputs {
		start := out.(map[string]any)["start"].(int)
		byStart[start] = append(byStart[start], name)
	}
	var starts []int
	for s := range byStart {
		starts = append(starts, s)
	}
	sort.Ints(starts)
	var groups [][]string
	for _, s := range starts {
		sort.Strings(byStart[s])
		groups = append(groups, byStart[s])
	}
	return groups
}

func altShape(typ string) map[string]any {
	return map[string]any{
		"ingest":  task(typ),
		"clean_a": task(typ, "ingest"),
		"clean_b": task(typ, "ingest"),
		"clean_c": task(typ, "ingest"),
		"merge":   task(typ, "clean_a", "clean_b"),
		"report":  task(typ, "merge", "clean_c"),
		"audit":   task(typ, "report"),
	}
}

func TestEngineLevelsAltShape(t *testing.T) {
	tar := casesTar(t)
	res, _, err := runStar(t, tar, "/cases.star", "levels_of", spec(nil, altShape("fake/ok")))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"levels": []any{
		[]any{"ingest"},
		[]any{"clean_a", "clean_b", "clean_c"},
		[]any{"merge"},
		[]any{"report"},
		[]any{"audit"},
	}}, toGo(t, res))
}

func TestEngineConcurrency(t *testing.T) {
	tar := casesTar(t)
	for _, tc := range []struct {
		name   string
		spec   map[string]any
		groups [][]string
	}{
		{
			name: "test_levels_alt_shape",
			spec: spec(map[string]any{"max_concurrency": 2}, altShape("fake/sleep")),
			groups: [][]string{
				{"ingest"}, {"clean_a", "clean_b"}, {"clean_c"}, {"merge"}, {"report"}, {"audit"},
			},
		},
		{
			name: "test_max_concurrency_defaults_to_unbounded",
			spec: spec(map[string]any{"max_concurrency": 0, "default_retry": nil}, map[string]any{
				"root": task("fake/sleep"),
				"a":    task("fake/sleep", "root"),
				"b":    task("fake/sleep", "root"),
				"c":    task("fake/sleep", "root"),
			}),
			groups: [][]string{{"root"}, {"a", "b", "c"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runEngine(t, tar, "run_case", tc.spec, map[string]any{})
			require.Empty(t, got.err)
			states := map[string]string{}
			for name := range tc.spec["tasks"].(map[string]any) {
				states[name] = succeeded
			}
			assert.Equal(t, states, got.states)
			assert.Equal(t, tc.groups, startGroups(t, got.result["outputs"].(map[string]any)))
		})
	}
}

// with returns a copy of task m with key set to v.
func with(m map[string]any, key string, v any) map[string]any {
	out := map[string]any{}
	for k, val := range m {
		out[k] = val
	}
	out[key] = v
	return out
}

func retry(maxAttempts int) map[string]any {
	return map[string]any{"max_attempts": maxAttempts, "initial_interval": nil}
}

// A factory runs before dispatch, so its failure stops the workflow with its
// own text instead of failing one task without a reason.
func TestEngineFactoryFailure(t *testing.T) {
	got := runEngine(t, casesTar(t), "run_case", spec(nil, map[string]any{
		"a": task("fake/ok"),
		"b": task("fake/bad_factory", "a"),
		"c": task("fake/ok", "a"),
	}), map[string]any{})
	assert.Equal(t, map[string]string{"a": succeeded}, got.states)
	assert.Equal(t, "fail: bad factory: b", got.err)
}

func TestEngineFactory(t *testing.T) {
	tar := casesTar(t)
	for _, tc := range []struct {
		name    string
		spec    map[string]any
		outputs map[string]any
	}{
		{
			name: "test_retry_replacement",
			spec: spec(map[string]any{"default_retry": retry(4)}, map[string]any{
				"a": with(task("fake/ok"), "retry", retry(2)),
				"b": task("fake/ok"),
				"c": with(task("fake/ok"), "retry", retry(1)),
			}),
			outputs: map[string]any{
				"a": with(okRecord("a"), "retry", 1),
				"b": with(okRecord("b"), "retry", 3),
				"c": with(okRecord("c"), "retry", 0),
			},
		},
		{
			name:    "test_retry_absent",
			spec:    spec(map[string]any{"max_concurrency": 0, "default_retry": nil}, map[string]any{"a": task("fake/ok")}),
			outputs: map[string]any{"a": okRecord("a")},
		},
		{
			name: "retry_zero",
			spec: spec(map[string]any{"default_retry": retry(3)}, map[string]any{
				"a": with(task("fake/ok"), "retry", retry(0)),
			}),
			outputs: map[string]any{"a": okRecord("a")},
		},
		{
			name: "retry_zero_default",
			spec: spec(map[string]any{"default_retry": retry(0)}, map[string]any{
				"a": task("fake/ok"),
			}),
			outputs: map[string]any{"a": okRecord("a")},
		},
		{
			name: "retry_no_capability",
			spec: spec(map[string]any{"default_retry": retry(3)}, map[string]any{
				"a": with(task("fake/noretry"), "retry", retry(2)),
			}),
			outputs: map[string]any{"a": map[string]any{"ep": "x.py", "alias": "a", "fields": map[string]any{}}},
		},
		{
			name: "factory_kwargs",
			spec: spec(nil, map[string]any{
				"a": with(with(task("fake/ok"),
					"config", map[string]any{"entry_point": "a.py", "args": []any{}, "kwargs": map[string]any{}, "lr": 0.5, "name": "n"}),
					"job_specs", map[string]any{"fake": map[string]any{"cpu": 2, "lr": 0.25}, "other": map[string]any{"x": 1}}),
			}),
			outputs: map[string]any{"a": map[string]any{
				"ep": "a.py", "alias": "a", "args": []any{}, "kwargs": map[string]any{}, "retry": "absent",
				"fields": map[string]any{"cpu": 2, "lr": 0.5, "name": "n"},
			}},
		},
		{
			name: "callable_args",
			spec: spec(nil, map[string]any{
				"a": with(task("fake/ok"), "config", map[string]any{"entry_point": "x.py", "args": []any{1, "two"}, "kwargs": map[string]any{"k": "v"}}),
			}),
			outputs: map[string]any{"a": with(with(okRecord("a"), "args", []any{1, "two"}), "kwargs", map[string]any{"k": "v"})},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runEngine(t, tar, "run_case", tc.spec, map[string]any{})
			require.Empty(t, got.err)
			states := map[string]any{}
			for name := range tc.outputs {
				states[name] = succeeded
			}
			assert.Equal(t, map[string]any{"states": states, "outputs": tc.outputs}, got.result)
		})
	}
}

// config returns task m with config set to entry_point x.py plus fields.
func config(m map[string]any, fields map[string]any) map[string]any {
	c := map[string]any{"entry_point": "x.py"}
	for k, v := range fields {
		c[k] = v
	}
	return with(m, "config", c)
}

func TestEngineRefs(t *testing.T) {
	tar := casesTar(t)
	params := map[string]any{
		"epochs": 3, "cpu": 2, "s": "x", "flag": true, "f": 1.5, "a": "{{ params.b }}", "b": "x",
		"big": 1234567.5, "small": 0.000015, "tiny": 1.5e-7, "neg": -0.00003, "frac": 123456789012.25,
	}
	for _, tc := range []struct {
		name     string
		spec     map[string]any
		states   map[string]string
		fields   map[string]map[string]any // task -> expected fields of its record
		args     map[string][]any          // task -> expected callable args
		kwargs   map[string]map[string]any // task -> expected callable kwargs
		messages map[string]string
		err      string
	}{
		{
			name:   "test_refs_non_recursive",
			spec:   spec(nil, map[string]any{"a": config(task("fake/ok"), map[string]any{"args": []any{"{{ params.a }}"}})}),
			states: map[string]string{"a": succeeded},
			args:   map[string][]any{"a": {"{{ params.b }}"}},
		},
		{
			name: "test_refs_in_nested_structures",
			spec: spec(nil, map[string]any{
				"root": task("fake/ok"),
				"child": config(task("fake/ok", "root"), map[string]any{"kwargs": map[string]any{
					"note": "s={{ params.s }} root={{ tasks.root.output.alias }}",
					"list": []any{"{{ params.epochs }}", 3},
					"deep": map[string]any{"inner": []any{map[string]any{"ep": "{{ tasks.root.output.ep }}"}}},
				}}),
			}),
			states: map[string]string{"root": succeeded, "child": succeeded},
			kwargs: map[string]map[string]any{"child": {
				"note": "s=x root=root",
				"list": []any{3, 3},
				"deep": map[string]any{"inner": []any{map[string]any{"ep": "x.py"}}},
			}},
		},
		{
			name:   "ref_config_scalar",
			spec:   spec(nil, map[string]any{"a": config(task("fake/ok"), map[string]any{"epochs": "{{ params.epochs }}"})}),
			states: map[string]string{"a": succeeded},
			fields: map[string]map[string]any{"a": {"epochs": 3}},
		},
		{
			name:   "ref_job_specs_slot",
			spec:   spec(nil, map[string]any{"a": with(task("fake/ok"), "job_specs", map[string]any{"fake": map[string]any{"cpu": "{{params.cpu}}"}})}),
			states: map[string]string{"a": succeeded},
			fields: map[string]map[string]any{"a": {"cpu": 2}},
		},
		{
			name: "ref_embedded",
			spec: spec(nil, map[string]any{
				"root": config(task("fake/ok"), map[string]any{"args": []any{1, "two"}}),
				"a": config(task("fake/ok", "root"), map[string]any{
					"n":      "n={{ params.epochs }}",
					"text":   "flag={{ params.flag }} f={{ params.f }} s={{ params.s }} args={{ tasks.root.output.args }}",
					"space":  " {{ params.epochs }}",
					"floats": "{{ params.big }} {{ params.small }} {{ params.tiny }} {{ params.neg }} {{ params.frac }}",
				}),
			}),
			states: map[string]string{"root": succeeded, "a": succeeded},
			fields: map[string]map[string]any{"a": {"n": "n=3", "text": `flag=true f=1.5 s=x args=[1,"two"]`, "space": " 3",
				"floats": "1234567.5 0.000015 1.5e-7 -0.00003 123456789012.25"}},
		},
		{
			name: "ref_deep_output",
			spec: spec(nil, map[string]any{
				"a": config(task("fake/ok"), map[string]any{"m": map[string]any{"k": "deep"}}),
				"b": config(task("fake/ok", "a"), map[string]any{"v": "{{ tasks.a.output.fields.m.k }}"}),
			}),
			states: map[string]string{"a": succeeded, "b": succeeded},
			fields: map[string]map[string]any{"b": {"v": "deep"}},
		},
		{
			name: "test_ref_unknown_param",
			spec: spec(nil, map[string]any{
				"root":    config(task("fake/ok"), map[string]any{"args": []any{"{{ params.nope }}"}}),
				"other":   task("fake/ok"),
				"after":   task("fake/ok", "root"),
				"handler": task("fake/ok", edge{"root", "OUTCOME_FAILED"}),
			}),
			states:   map[string]string{"root": failed, "other": succeeded, "after": skipped, "handler": succeeded},
			messages: map[string]string{"root": `Failed: config.args[0]: reference "{{ params.nope }}" names an unknown parameter`, "after": "Skipped: dependency gate not satisfied"},
			err:      "declarative: failed task(s): root",
		},
		{
			name: "test_ref_non_dependency",
			spec: spec(nil, map[string]any{
				"root":  task("fake/ok"),
				"other": task("fake/ok"),
				"child": config(task("fake/ok", "root"), map[string]any{"args": []any{"{{ tasks.other.output.alias }}"}}),
			}),
			states:   map[string]string{"root": succeeded, "other": succeeded, "child": failed},
			messages: map[string]string{"child": `Failed: config.args[0]: reference "{{ tasks.other.output.alias }}" names task "other", which is not a dependency`},
			err:      "declarative: failed task(s): child",
		},
		{
			name: "test_ref_missing_output_field",
			spec: spec(nil, map[string]any{
				"root":  task("fake/ok"),
				"child": with(task("fake/ok", "root"), "job_specs", map[string]any{"fake": map[string]any{"cpu": "{{ tasks.root.output.fields.nope }}"}}),
			}),
			states:   map[string]string{"root": succeeded, "child": failed},
			messages: map[string]string{"child": `Failed: job_specs.fake.cpu: reference "{{ tasks.root.output.fields.nope }}" names field "nope", which the output does not have`},
			err:      "declarative: failed task(s): child",
		},
		{
			name: "test_ref_output_of_failed_task",
			spec: spec(nil, map[string]any{
				"a": task("fake/fail"),
				"b": config(task("fake/ok", edge{"a", "OUTCOME_FAILED"}), map[string]any{"args": []any{"{{ tasks.a.output.alias }}"}}),
			}),
			states:   map[string]string{"a": failed, "b": failed},
			messages: map[string]string{"b": `Failed: config.args[0]: reference "{{ tasks.a.output.alias }}" names task "a", which did not succeed`},
			err:      "declarative: failed task(s): a, b",
		},
		{
			name: "ref_malformed",
			spec: spec(nil, map[string]any{
				"a": config(task("fake/ok"), map[string]any{"x": "{{ params.Bad }}"}),
				"b": config(task("fake/ok"), map[string]any{"x": "n={{ params.epochs"}),
				"c": config(task("fake/ok"), map[string]any{"x": "{{ tasks.a }}"}),
				"d": config(task("fake/ok"), map[string]any{"x": "{{ pipeline.execution_time }}"}),
			}),
			states: map[string]string{"a": failed, "b": failed, "c": failed, "d": failed},
			messages: map[string]string{
				"a": `Failed: config.x: reference "{{ params.Bad }}" has a malformed name`,
				"b": `Failed: config.x: unterminated "{{" in "n={{ params.epochs"`,
				"c": `Failed: config.x: reference "{{ tasks.a }}" must name "tasks.a.output"`,
				"d": `Failed: config.x: reference "{{ pipeline.execution_time }}" must start with "params." or "tasks."`,
			},
			err: "declarative: failed task(s): a, b, c, d",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runEngine(t, tar, "run_case", tc.spec, params)
			assert.Equal(t, tc.states, got.states, "run view")
			if tc.messages != nil {
				assert.Equal(t, tc.messages, got.messages)
			}
			if tc.err != "" {
				assert.Equal(t, "fail: "+tc.err, got.err)
				return
			}
			require.Empty(t, got.err)
			outputs := got.result["outputs"].(map[string]any)
			for name, want := range tc.fields {
				assert.Equal(t, want, outputs[name].(map[string]any)["fields"], name)
			}
			for name, want := range tc.args {
				assert.Equal(t, want, outputs[name].(map[string]any)["args"], name)
			}
			for name, want := range tc.kwargs {
				assert.Equal(t, want, outputs[name].(map[string]any)["kwargs"], name)
			}
		})
	}
}

func TestEngineNumberParam(t *testing.T) {
	tar := casesTar(t)
	s := spec(nil, map[string]any{
		"a": config(task("fake/types"), map[string]any{
			"lr":     "{{ params.lr }}",
			"epochs": "{{ params.epochs }}",
			"text":   "lr={{ params.lr }} zero={{ params.zero }}",
		}),
	})
	s["parameters"] = map[string]any{
		"lr":     map[string]any{"type": "PARAMETER_TYPE_NUMBER", "default": 1, "required": false, "description": ""},
		"epochs": map[string]any{"type": "PARAMETER_TYPE_INTEGER", "default": 3, "required": false, "description": ""},
		"zero":   map[string]any{"type": "PARAMETER_TYPE_NUMBER", "default": 0, "required": false, "description": ""},
	}
	got := runEngine(t, tar, "run_case", s, map[string]any{"lr": 1, "epochs": 3, "zero": 0})
	require.Empty(t, got.err)
	assert.Equal(t, map[string]any{
		"types":  map[string]any{"lr": "float", "epochs": "int", "text": "string"},
		"fields": map[string]any{"lr": 1.0, "epochs": 3, "text": "lr=1 zero=0"},
	}, got.result["outputs"].(map[string]any)["a"])
}
