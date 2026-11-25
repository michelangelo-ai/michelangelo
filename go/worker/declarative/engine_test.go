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
	return map[string]any{"ep": "x.py", "alias": alias, "args": []any{}, "kwargs": map[string]any{}, "retry": nil, "fields": map[string]any{}}
}

type engineRun struct {
	result map[string]any
	// states is the run view: the last state each task reported.
	states map[string]string
	err    string
}

func runEngine(t *testing.T, tar []byte, fn string, args ...any) engineRun {
	t.Helper()
	res, progress, err := runStar(t, tar, "/cases.star", fn, args...)
	out := engineRun{states: map[string]string{}}
	for _, line := range progress {
		var p struct {
			TaskName  string `json:"task_name"`
			TaskState string `json:"task_state"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &p), line)
		out.states[p.TaskName] = p.TaskState
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
