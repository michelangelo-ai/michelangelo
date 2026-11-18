package declarative

import (
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Plan validates w against release rel and returns its explain plan. The
// explain plan is a dry-run preview of how the interpreter runs w. Plan does
// not run any task or contact any service.
//
// The plan holds:
//   - Levels: the tasks grouped by depth in the depends_on graph. Level 0 holds
//     tasks with no dependencies. Each other task is one level below its
//     deepest dependency. The tasks in one level do not depend on each other,
//     so the interpreter can run them in parallel. Tasks in a level are sorted
//     by name.
//   - TaskCount: the number of tasks in w.
//   - InterpreterVersion: rel.Version, the interpreter release that runs w.
//   - Warnings: a copy of warnings, which the caller collects.
//
// The levels show the order of dependencies only. A task whose depends_on
// gates are not met at run time is skipped, but it still appears in the plan.
//
// Plan returns the first validation error, as ValidateAgainst does.
func Plan(w *v2.DeclarativeWorkflow, lim Limits, rel *Release, warnings []string) (*v2.ExplainPlan, error) {
	if err := ValidateAgainst(w, lim, rel); err != nil {
		return nil, err
	}
	g, err := buildGraph(w)
	if err != nil {
		return nil, err
	}
	p := &v2.ExplainPlan{
		InterpreterVersion: rel.Version,
		TaskCount:          uint32(len(w.Tasks)),
		Warnings:           append([]string(nil), warnings...),
	}
	for _, lvl := range g.levels() {
		p.Levels = append(p.Levels, &v2.ExecutionLevel{Tasks: lvl})
	}
	return p, nil
}
