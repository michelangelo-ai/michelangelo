//go:generate mamockgen Context
package context

import "time"

// Context interface that we want to mock - this is the same as context.Context
type Context interface {
	// Deadline returns the time when work done on behalf of this context
	// should be canceled.
	Deadline() (deadline time.Time, ok bool)

	// Done returns a channel that's closed when work done on behalf of this
	// context should be canceled.
	Done() <-chan struct{}

	// Err returns a non-nil error value after Done is closed.
	Err() error

	// Value returns the value associated with this context for key.
	Value(key interface{}) interface{}
}
