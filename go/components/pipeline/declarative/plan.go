package declarative

import (
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Plan runs ValidateAgainst, then builds the dry-run explain plan for rel.
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
