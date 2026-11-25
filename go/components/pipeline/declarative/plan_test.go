package declarative

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func altShape() *v2pb.DeclarativeWorkflow {
	return newWorkflow(map[string]*v2pb.DeclarativeTask{
		"a": task("core/python"),
		"b": task("core/python", "a"),
		"c": task("core/python"),
		"d": task("core/python", "b", "c"),
		"e": task("core/python"),
	})
}

func diamondFailure() *v2pb.DeclarativeWorkflow {
	w := diamond()
	notify := task("core/python")
	notify.DependsOn = []*v2pb.TaskDependency{{Task: "publish", When: v2pb.OUTCOME_FAILED}}
	cleanup := task("core/python")
	cleanup.DependsOn = []*v2pb.TaskDependency{{Task: "train", When: v2pb.OUTCOME_COMPLETED}}
	w.Tasks["notify"] = notify
	w.Tasks["cleanup"] = cleanup
	return w
}

func TestPlanGolden(t *testing.T) {
	cases := []struct {
		name     string
		warnings []string
	}{
		// diamond: one root, a fan-out of two, and a fan-in.
		{"diamond", nil},
		// alt_shape: roots a, c, and e at level 0; d joins a depth-1 and a
		// depth-0 dependency. Plan copies the caller's warnings unchanged.
		{"alt_shape", []string{"caller-supplied warning"}},
		// diamond_failure: FAILED and COMPLETED edges still order tasks into
		// levels. Each one warns, because the upstream types lack outcome_mode.
		{"diamond_failure", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := readWorkflow(t, filepath.Join("testdata", "specs", c.name+".json"))
			got, err := Plan(w, Limits{}, loadRelease(t), c.warnings)
			require.NoError(t, err)
			f, err := os.Open(filepath.Join("testdata", "plans", c.name+".json"))
			require.NoError(t, err)
			defer f.Close()
			want := &v2pb.ExplainPlan{}
			require.NoError(t, jsonpb.Unmarshal(f, want))
			assert.Truef(t, gogoproto.Equal(want, got), "explain-plan mismatch\nwant: %s\ngot:  %s", want.String(), got.String())
		})
	}
}

func TestPlanWarnsInitialInterval(t *testing.T) {
	w := typedDiamond()
	w.Tasks["train"].Retry = &v2pb.RetryPolicy{MaxAttempts: 2, InitialInterval: &types.Duration{Seconds: 30}}
	w.WorkflowConfig = &v2pb.WorkflowSettings{DefaultRetry: &v2pb.RetryPolicy{MaxAttempts: 1, InitialInterval: &types.Duration{Seconds: 5}}}
	p, err := Plan(w, Limits{}, loadRelease(t), []string{"caller"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"caller",
		"tasks.train.retry.initial_interval is not used yet",
		"workflow_config.default_retry.initial_interval is not used yet",
	}, p.Warnings)
}

func TestPlanWarningOrder(t *testing.T) {
	w := typedDiamond()
	w.Tasks["publish"].DependsOn = []*v2pb.TaskDependency{{Task: "train"}, {Task: "evaluate", When: v2pb.OUTCOME_COMPLETED}}
	w.Tasks["publish"].Retry = &v2pb.RetryPolicy{InitialInterval: &types.Duration{Seconds: 1}}
	w.Tasks["evaluate"].Retry = &v2pb.RetryPolicy{InitialInterval: &types.Duration{Seconds: 1}}
	p, err := Plan(w, Limits{}, loadRelease(t), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"tasks.evaluate.retry.initial_interval is not used yet",
		"tasks.publish.depends_on[1]: the handler runs, but it gets no error text until outcome mode ships",
		"tasks.publish.retry.initial_interval is not used yet",
	}, p.Warnings)
}

// The warning depends on the upstream type: its factory is what would hand
// the error text to the handler.
func TestPlanNoWarningForOutcomeMode(t *testing.T) {
	w := typedDiamond()
	w.Tasks["publish"].TaskFunction = "core/outcome"
	notify := task("core/python")
	notify.DependsOn = []*v2pb.TaskDependency{{Task: "publish", When: v2pb.OUTCOME_FAILED}}
	notify.Config.Fields["entry_point"] = strVal("main.py")
	w.Tasks["notify"] = notify
	p, err := Plan(w, Limits{}, loadRelease(t), nil)
	require.NoError(t, err)
	assert.Empty(t, p.Warnings)

	// The handler type does not matter: an outcome_mode handler on a plain
	// upstream still warns.
	w = typedDiamond()
	handler := task("core/outcome")
	handler.DependsOn = []*v2pb.TaskDependency{{Task: "publish", When: v2pb.OUTCOME_FAILED}}
	handler.Config.Fields["entry_point"] = strVal("main.py")
	w.Tasks["notify"] = handler
	p, err = Plan(w, Limits{}, loadRelease(t), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"tasks.notify.depends_on[0]: the handler runs, but it gets no error text until outcome mode ships"}, p.Warnings)
}

func TestPlanRejectsCycle(t *testing.T) {
	w := typed(newWorkflow(map[string]*v2pb.DeclarativeTask{"a": task("core/python", "b"), "b": task("core/python", "a")}))
	_, err := Plan(w, Limits{}, loadRelease(t), nil)
	assert.EqualError(t, err, "tasks form a depends_on cycle: a -> b -> a")
}

func TestPlanCopiesWarnings(t *testing.T) {
	in := []string{"w1"}
	p, err := Plan(typedDiamond(), Limits{}, loadRelease(t), in)
	require.NoError(t, err)
	in[0] = "changed"
	assert.Equal(t, []string{"w1"}, p.Warnings)
}

func TestPlanRunsFullValidation(t *testing.T) {
	rel := loadRelease(t)
	noVersion := typedDiamond()
	noVersion.SchemaVersion = ""
	badRef := typedDiamond()
	badRef.Tasks["train"].Config.Fields["epochs"] = strVal("{{ params.nope }}")
	badType := typedDiamond()
	badType.Tasks["train"].TaskFunction = "core/spark"
	cases := []struct {
		name string
		w    *v2pb.DeclarativeWorkflow
		lim  Limits
		want string
	}{
		{"schema version", noVersion, Limits{}, `schema_version "" is not supported; use "v1alpha1"`},
		{"task limit", typedDiamond(), Limits{MaxTasks: 3}, "tasks has 4 entries; the limit is 3"},
		{"undeclared parameter", badRef, Limits{}, `task "train": config.epochs references undeclared parameter "nope"`},
		{"unknown task type", badType, Limits{}, `task "train": task_function "core/spark" is not a task type in core-v1.4.2`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Plan(c.w, c.lim, rel, nil)
			assert.EqualError(t, err, c.want)
		})
	}
}
