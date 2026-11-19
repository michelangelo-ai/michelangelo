package declarative

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var taskNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// graph is the depends_on graph of a workflow. deps maps a task to the tasks
// it depends on, and rdeps maps a task to the tasks that depend on it.
type graph struct {
	names []string
	deps  map[string][]string
	rdeps map[string][]string
	// okDeps holds only the edges that run when the dependency succeeded.
	okDeps map[string][]string
}

// buildGraph checks task names and depends_on edges, then builds the graph.
// It rejects unknown, duplicate, and self edges, unknown when values, and cycles.
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
			switch d.When {
			case v2.OUTCOME_UNSPECIFIED, v2.OUTCOME_SUCCEEDED, v2.OUTCOME_FAILED, v2.OUTCOME_COMPLETED:
			default:
				return nil, &Error{Task: name, Msg: fmt.Sprintf("depends_on[%d].when %d is not a known outcome; use SUCCEEDED, FAILED, COMPLETED, or leave it unset", i, d.When)}
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
	if cyc := g.cyclePath(); len(cyc) > 0 {
		return nil, &Error{Msg: "tasks form a depends_on cycle: " + strings.Join(cyc, " -> ")}
	}
	return g, nil
}

// cyclePath returns one dependency cycle, first task repeated at the end, or
// nil when the graph is acyclic.
func (g *graph) cyclePath() []string {
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
	// Every task left with indegree > 0 has a dependency that is also left, so
	// a walk through left tasks must revisit one.
	var start string
	for _, n := range g.names {
		if indeg[n] > 0 {
			start = n
			break
		}
	}
	pos := map[string]int{}
	var path []string
	for n := start; ; {
		if i, seen := pos[n]; seen {
			return append(path[i:], n)
		}
		pos[n] = len(path)
		path = append(path, n)
		for _, d := range g.deps[n] {
			if indeg[d] > 0 {
				n = d
				break
			}
		}
	}
}

// levels groups tasks by depth: level 0 holds tasks with no dependencies, and
// each other task sits one level below its deepest dependency. Tasks in one
// level do not depend on each other.
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

// reachability holds, for each task, bitsets over g.names of the tasks it
// depends on, directly or not, and of the tasks that have succeeded whenever
// it runs.
type reachability struct {
	idx         map[string]int
	trans, succ [][]uint64
}

// reachability computes both sets for every task in one pass in level order.
// A skipped task satisfies no gate, so every transitive dependency of a
// running task ran, and each success edge out of those tasks proves that its
// target succeeded:
//
//	trans(n) = union over deps d of {d} and trans(d)
//	succ(n)  = okDeps(n), plus the union over deps d of succ(d)
func (g *graph) reachability() *reachability {
	r := &reachability{idx: make(map[string]int, len(g.names))}
	for i, n := range g.names {
		r.idx[n] = i
	}
	words := (len(g.names) + 63) / 64
	r.trans = make([][]uint64, len(g.names))
	r.succ = make([][]uint64, len(g.names))
	for _, lvl := range g.levels() {
		for _, n := range lvl {
			t, s := make([]uint64, words), make([]uint64, words)
			for _, d := range g.deps[n] {
				j := r.idx[d]
				t[j/64] |= 1 << (j % 64)
				for w := range t {
					t[w] |= r.trans[j][w]
					s[w] |= r.succ[j][w]
				}
			}
			for _, d := range g.okDeps[n] {
				j := r.idx[d]
				s[j/64] |= 1 << (j % 64)
			}
			r.trans[r.idx[n]], r.succ[r.idx[n]] = t, s
		}
	}
	return r
}

// dependsOn reports whether task depends on target, directly or not.
func (r *reachability) dependsOn(task, target string) bool {
	return has(r.trans[r.idx[task]], r.idx[target])
}

// succeeded reports whether target has succeeded whenever task runs.
func (r *reachability) succeeded(task, target string) bool {
	return has(r.succ[r.idx[task]], r.idx[target])
}

// has reports whether bit i is set in b.
func has(b []uint64, i int) bool { return b[i/64]&(1<<(i%64)) != 0 }
