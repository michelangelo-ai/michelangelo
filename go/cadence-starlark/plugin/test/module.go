package test

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/temporal"
)

type Module struct{}

var _ starlark.HasAttrs = &Module{}

func (f *Module) String() string                        { return "test" }
func (f *Module) Type() string                          { return "test" }
func (f *Module) Freeze()                               {}
func (f *Module) Truth() starlark.Bool                  { return true }
func (f *Module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *Module) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *Module) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var builtins = map[string]*starlark.Builtin{
	"true":      starlark.NewBuiltin("true", _true),
	"false":     starlark.NewBuiltin("false", _false),
	"equal":     starlark.NewBuiltin("equal", _equal),
	"not_equal": starlark.NewBuiltin("not_equal", _notEqual),
}

var properties = map[string]star.PropertyFactory{}

func _true(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	var message starlark.Value
	if err := starlark.UnpackArgs("true", args, kwargs, "v", &v, "message?", &message); err != nil {
		return nil, err
	}
	if !v.Truth() {
		code := "assert"
		if message != nil {
			code = fmt.Sprintf("%s: %s", code, message.String())
		}
		return nil, temporal.NewApplicationError(code, code)
	}
	return starlark.None, nil
}

func _false(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	var message starlark.Value
	if err := starlark.UnpackArgs("false", args, kwargs, "v", &v, "message?", &message); err != nil {
		return nil, err
	}
	if v.Truth() {
		code := "assert"
		if message != nil {
			code = fmt.Sprintf("%s: %s", code, message.String())
		}
		return nil, temporal.NewApplicationError(code, code)
	}
	return starlark.None, nil
}

func _equal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var expected, actual starlark.Value
	var message starlark.Value
	if err := starlark.UnpackArgs("equal", args, kwargs, "expected", &expected, "actual", &actual, "message?", &message); err != nil {
		return nil, err
	}
	if eq, err := starlark.Equal(expected, actual); err != nil {
		return nil, err
	} else if !eq {
		code := "assert"
		if message != nil {
			code = fmt.Sprintf("%s: %s", code, message.String())
		}
		code = fmt.Sprintf("%s\nExpected : %s\nActual   : %s", code, expected.String(), actual.String())
		return nil, temporal.NewApplicationError(code, code)
	}
	return starlark.None, nil
}

func _notEqual(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var expected, actual starlark.Value
	var message starlark.Value
	if err := starlark.UnpackArgs("not_equal", args, kwargs, "expected", &expected, "actual", &actual, "message?", &message); err != nil {
		return nil, err
	}
	if eq, err := starlark.Equal(expected, actual); err != nil {
		return nil, err
	} else if eq {
		code := "assert"
		if message != nil {
			code = fmt.Sprintf("%s: %s", code, message.String())
		}
		code = fmt.Sprintf("%s\nExpected : %s\nActual   : %s", code, expected.String(), actual.String())
		return nil, temporal.NewApplicationError(code, code)
	}
	return starlark.None, nil
}
