// Package declarative decodes, validates, and plans a DeclarativeWorkflow.
package declarative

import (
	"fmt"
	"strings"
)

// Error is one validation failure.
type Error struct {
	Task    string
	Param   string
	Field   string
	Release string
	Msg     string
}

// Error formats the failure as "task|parameter <name>: <field> <msg> in <release>",
// leaving out the parts that are empty.
func (e *Error) Error() string {
	var b strings.Builder
	switch {
	case e.Task != "":
		fmt.Fprintf(&b, "task %q: ", e.Task)
	case e.Param != "":
		fmt.Fprintf(&b, "parameter %q: ", e.Param)
	}
	if e.Field != "" {
		b.WriteString(e.Field)
		b.WriteByte(' ')
	}
	b.WriteString(e.Msg)
	if e.Release != "" {
		b.WriteString(" in ")
		b.WriteString(e.Release)
	}
	return b.String()
}

// Limits bounds a workflow. A zero field takes the default.
type Limits struct {
	// MaxTasks bounds the number of tasks.
	MaxTasks int
	// MaxDependencies bounds the total number of depends_on entries.
	MaxDependencies int
}

// DefaultLimits returns the platform defaults.
func DefaultLimits() Limits { return Limits{MaxTasks: 500, MaxDependencies: 5000} }

// withDefaults replaces each zero or negative limit with its default.
func (l Limits) withDefaults() Limits {
	if l.MaxTasks <= 0 {
		l.MaxTasks = DefaultLimits().MaxTasks
	}
	if l.MaxDependencies <= 0 {
		l.MaxDependencies = DefaultLimits().MaxDependencies
	}
	return l
}
