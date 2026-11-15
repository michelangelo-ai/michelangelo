package declarative

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func altShape() *v2.DeclarativeWorkflow {
	return newWorkflow(map[string]*v2.DeclarativeTask{
		"a": task("core/python"),
		"b": task("core/python", "a"),
		"c": task("core/python"),
		"d": task("core/python", "b", "c"),
		"e": task("core/python"),
	})
}

func diamondFailure() *v2.DeclarativeWorkflow {
	w := diamond()
	notify := task("core/python")
	notify.DependsOn = []*v2.TaskDependency{{Task: "publish", When: v2.OUTCOME_FAILED}}
	cleanup := task("core/python")
	cleanup.DependsOn = []*v2.TaskDependency{{Task: "train", When: v2.OUTCOME_COMPLETED}}
	w.Tasks["notify"] = notify
	w.Tasks["cleanup"] = cleanup
	return w
}

func TestPlanGolden(t *testing.T) {
	cases := []struct {
		name     string
		w        *v2.DeclarativeWorkflow
		warnings []string
	}{
		{"diamond", typed(diamond()), nil},
		{"alt_shape", typed(altShape()), []string{`task "e" has no dependents and no dependencies`}},
		{"diamond_failure", typed(diamondFailure()), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Plan(c.w, Limits{}, loadRelease(t), c.warnings)
			require.NoError(t, err)
			f, err := os.Open(filepath.Join("testdata", "plans", c.name+".json"))
			require.NoError(t, err)
			defer f.Close()
			want := &v2.ExplainPlan{}
			require.NoError(t, jsonpb.Unmarshal(f, want))
			assert.Truef(t, gogoproto.Equal(want, got), "explain-plan mismatch\nwant: %s\ngot:  %s", want.String(), got.String())
		})
	}
}

func TestPlanRejectsCycle(t *testing.T) {
	w := typed(newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python", "b"), "b": task("core/python", "a")}))
	_, err := Plan(w, Limits{}, loadRelease(t), nil)
	assert.EqualError(t, err, "tasks form a cycle: a, b")
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
		w    *v2.DeclarativeWorkflow
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
