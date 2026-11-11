package declarative

import (
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Plan builds the dry-run explain-plan: topological levels, task count, and
// the warnings the caller collected. It fails on a graph that does not
// validate structurally.
func Plan(w *v2.DeclarativeWorkflow, interpreterVersion string, warnings []string) (*v2.ExplainPlan, error) {
	if w == nil {
		return nil, &Error{Msg: "workflow is not set"}
	}
	if len(w.Tasks) == 0 {
		return nil, &Error{Msg: "tasks must have at least one entry"}
	}
	g, err := buildGraph(w)
	if err != nil {
		return nil, err
	}
	p := &v2.ExplainPlan{
		InterpreterVersion: interpreterVersion,
		TaskCount:          uint32(len(w.Tasks)),
		Warnings:           append([]string(nil), warnings...),
	}
	for _, lvl := range g.levels() {
		p.Levels = append(p.Levels, &v2.ExecutionLevel{Tasks: lvl})
	}
	return p, nil
}
