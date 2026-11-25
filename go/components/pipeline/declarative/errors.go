package declarative

import (
	"fmt"
	"strings"
)

// Error is one problem in the content of a workflow. Decode, Validate,
// ValidateAgainst, and Plan return *Error for every such problem, so a server
// can map *Error to InvalidArgument. Any other error from this package is an
// internal fault or comes from LoadReleases or ParseRelease, which report
// problems in release files as plain errors.
type Error struct {
	// Task is the name of the task at fault, or "".
	Task string
	// Param is the name of the parameter at fault, or "". Error ignores it
	// when Task is set.
	Param string
	// Field is the path of the value at fault inside the task, such as
	// "config.ray.image", or "".
	Field string
	// Release is the version of the release whose schema rejected the task,
	// or "".
	Release string
	// Msg says what is wrong and, where it can, how to fix it.
	Msg string
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
