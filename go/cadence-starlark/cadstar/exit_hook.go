package cadstar

import (
	"go.starlark.net/starlark"
	"go.uber.org/multierr"
)

type exitHook struct {
	fn     starlark.Callable
	args   starlark.Tuple
	kwargs []starlark.Tuple
}

type ExitHooks struct {
	hooks []*exitHook
	len   int
}

func (r *ExitHooks) Register(fn starlark.Callable, args starlark.Tuple, kwargs []starlark.Tuple) {
	h := &exitHook{fn: fn, args: args, kwargs: kwargs}
	r.hooks = append(r.hooks, h)
	r.len += 1
}

func (r *ExitHooks) Unregister(fn starlark.Callable) {
	for i, h := range r.hooks {
		if h != nil && h.fn == fn {
			r.hooks[i] = nil
			r.len -= 1
		}
	}
}

func (r *ExitHooks) Run(t *starlark.Thread) error {
	hooks := r.hooks
	var _errors []error
	for i := len(hooks) - 1; i >= 0; i-- {
		if hooks[i] != nil {
			h := hooks[i]
			if _, err := starlark.Call(t, h.fn, h.args, h.kwargs); err != nil {
				_errors = append(_errors, err)
			}
		}
	}
	return multierr.Combine(_errors...)
}

func (r *ExitHooks) Len() int {
	return r.len
}
