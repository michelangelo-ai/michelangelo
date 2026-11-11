package declarative

import (
	"fmt"
	"strings"
	"testing"

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
