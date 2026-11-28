package declarative

import (
	"fmt"
	"sort"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
//   - Warnings: a copy of warnings, which the caller collects, then the
//     warnings that Plan adds for features the interpreter does not use yet.
//     Plan adds them per task in sorted name order: each depends_on entry by
//     index, then retry.initial_interval. Then it adds
//     workflow_config.default_retry.initial_interval.
//
// The levels show the order of dependencies only. A task whose depends_on
// gates are not met at run time is skipped, but it still appears in the plan.
//
// Plan returns the first validation error, as ValidateAgainst does.
func Plan(w *v2pb.DeclarativeWorkflow, lim Limits, rel *Release, warnings []string) (*v2pb.ExplainPlan, error) {
	g, err := validateAgainst(w, lim, rel)
	if err != nil {
		return nil, err
	}
	p := &v2pb.ExplainPlan{
		InterpreterVersion: rel.Version,
		TaskCount:          uint32(len(w.Tasks)),
		Warnings:           append(append([]string(nil), warnings...), unsupportedWarnings(w, rel)...),
	}
	for _, lvl := range g.levels() {
		p.Levels = append(p.Levels, &v2pb.ExecutionLevel{Tasks: lvl})
	}
	return p, nil
}

func unsupportedWarnings(w *v2pb.DeclarativeWorkflow, rel *Release) []string {
	names := make([]string, 0, len(w.Tasks))
	for n := range w.Tasks {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		t := w.Tasks[n]
		for i, d := range t.DependsOn {
			if d.When != v2pb.OUTCOME_FAILED && d.When != v2pb.OUTCOME_COMPLETED {
				continue
			}
			upstream := rel.TaskTypes[w.Tasks[d.Task].TaskFunction]
			if !upstream.HasCapability("outcome_mode") {
				out = append(out, fmt.Sprintf("tasks.%s.depends_on[%d]: the handler runs, but it gets no error text until outcome mode ships", n, i))
			}
		}
		if t.Retry.GetInitialInterval() != nil {
			out = append(out, fmt.Sprintf("tasks.%s.retry.initial_interval is not used yet", n))
		}
	}
	if w.WorkflowConfig.GetDefaultRetry().GetInitialInterval() != nil {
		out = append(out, "workflow_config.default_retry.initial_interval is not used yet")
	}
	return out
}
