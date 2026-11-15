package declarative

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var taskNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type graph struct {
	names []string
	deps  map[string][]string
	rdeps map[string][]string
	// okDeps holds only the edges that run when the dependency succeeded.
	okDeps map[string][]string
}

func buildGraph(w *v2.DeclarativeWorkflow) (*graph, error) {
	g := &graph{deps: map[string][]string{}, rdeps: map[string][]string{}, okDeps: map[string][]string{}}
	for name := range w.Tasks {
		g.names = append(g.names, name)
	}
	sort.Strings(g.names)
	for _, name := range g.names {
		if !taskNameRe.MatchString(name) {
			return nil, &Error{Task: name, Msg: "name must match " + taskNameRe.String()}
		}
		t := w.Tasks[name]
		if t == nil {
			return nil, &Error{Task: name, Msg: "task is not set"}
		}
		if t.TaskFunction == "" {
			return nil, &Error{Task: name, Msg: "task_function is not set"}
		}
	}
	for _, name := range g.names {
		seen := map[string]bool{}
		for i, d := range w.Tasks[name].DependsOn {
			if d == nil || d.Task == "" {
				return nil, &Error{Task: name, Msg: fmt.Sprintf("depends_on[%d].task is not set", i)}
			}
			if d.Task == name {
				return nil, &Error{Task: name, Msg: "depends_on names the task itself"}
			}
			if _, ok := w.Tasks[d.Task]; !ok {
				return nil, &Error{Task: name, Msg: fmt.Sprintf("depends_on names unknown task %q", d.Task)}
			}
			if _, ok := v2.TaskDependency_Outcome_name[int32(d.When)]; !ok {
				return nil, &Error{Task: name, Msg: fmt.Sprintf("depends_on[%d].when %d is not a known outcome", i, d.When)}
			}
			if seen[d.Task] {
				return nil, &Error{Task: name, Msg: fmt.Sprintf("depends_on names %q more than once", d.Task)}
			}
			seen[d.Task] = true
			g.deps[name] = append(g.deps[name], d.Task)
			g.rdeps[d.Task] = append(g.rdeps[d.Task], name)
			if d.When == v2.OUTCOME_UNSPECIFIED || d.When == v2.OUTCOME_SUCCEEDED {
				g.okDeps[name] = append(g.okDeps[name], d.Task)
			}
		}
		sort.Strings(g.deps[name])
	}
	for k := range g.rdeps {
		sort.Strings(g.rdeps[k])
	}
	if cyc := g.cycleMembers(); len(cyc) > 0 {
		return nil, &Error{Msg: "tasks form a cycle: " + strings.Join(cyc, ", ")}
	}
	return g, nil
}

func (g *graph) cycleMembers() []string {
	indeg := map[string]int{}
	for _, n := range g.names {
		indeg[n] = len(g.deps[n])
	}
	queue := []string{}
	for _, n := range g.names {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	done := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		done++
		for _, m := range g.rdeps[n] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if done == len(g.names) {
		return nil
	}
	var cyc []string
	for _, n := range g.names {
		if indeg[n] > 0 {
			cyc = append(cyc, n)
		}
	}
	return cyc
}

func (g *graph) levels() [][]string {
	depth := map[string]int{}
	var visit func(string) int
	visit = func(n string) int {
		if d, ok := depth[n]; ok {
			return d
		}
		d := 0
		for _, p := range g.deps[n] {
			if pd := visit(p) + 1; pd > d {
				d = pd
			}
		}
		depth[n] = d
		return d
	}
	deepest := -1
	for _, n := range g.names {
		if d := visit(n); d > deepest {
			deepest = d
		}
	}
	out := make([][]string, deepest+1)
	for _, n := range g.names {
		out[depth[n]] = append(out[depth[n]], n)
	}
	return out
}

func (g *graph) transitiveDeps(task string) map[string]bool { return reach(g.deps, task) }

// succeededDeps returns the tasks that must have succeeded before task runs.
func (g *graph) succeededDeps(task string) map[string]bool { return reach(g.okDeps, task) }

func reach(adj map[string][]string, task string) map[string]bool {
	out := map[string]bool{}
	stack := append([]string{}, adj[task]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if out[n] {
			continue
		}
		out[n] = true
		stack = append(stack, adj[n]...)
	}
	return out
}
