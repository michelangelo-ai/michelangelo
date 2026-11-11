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
		{"diamond", diamond(), nil},
		{"alt_shape", altShape(), []string{`task "e" has no dependents and no dependencies`}},
		{"diamond_failure", diamondFailure(), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, Validate(c.w, Limits{}))
			got, err := Plan(c.w, "core-v1.4.2", c.warnings)
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
	w := newWorkflow(map[string]*v2.DeclarativeTask{"a": task("core/python", "b"), "b": task("core/python", "a")})
	_, err := Plan(w, "core-v1.4.2", nil)
	assert.EqualError(t, err, "tasks form a cycle: a, b")
}

func TestPlanCopiesWarnings(t *testing.T) {
	in := []string{"w1"}
	p, err := Plan(diamond(), "v", in)
	require.NoError(t, err)
	in[0] = "changed"
	assert.Equal(t, []string{"w1"}, p.Warnings)
}
