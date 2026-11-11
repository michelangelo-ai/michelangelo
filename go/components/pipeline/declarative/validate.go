package declarative

import (
	"fmt"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Validate runs the checks that need no task-type registry. It returns the
// first failure in step order.
func Validate(w *v2.DeclarativeWorkflow, lim Limits) error {
	lim = lim.withDefaults()
	if w == nil {
		return &Error{Msg: "workflow is not set"}
	}
	if w.SchemaVersion != SchemaVersion {
		return &Error{Msg: fmt.Sprintf("schema_version %q is not supported; use %q", w.SchemaVersion, SchemaVersion)}
	}
	if len(w.Tasks) == 0 {
		return &Error{Msg: "tasks must have at least one entry"}
	}
	if len(w.Tasks) > lim.MaxTasks {
		return &Error{Msg: fmt.Sprintf("tasks has %d entries; the limit is %d", len(w.Tasks), lim.MaxTasks)}
	}
	g, err := buildGraph(w)
	if err != nil {
		return err
	}
	_ = g
	return nil
}
