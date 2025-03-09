package star

import (
	"fmt"
	"go.starlark.net/starlark"
)

var CallableObjectBuiltin = starlark.NewBuiltin(CallableObjectType, MakeCallableObject)

const CallableObjectType = "callable_object"

func MakeCallableObject(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var fn starlark.Callable
	if err := starlark.UnpackArgs(CallableObjectType, args, kwargs, "fn", &fn); err != nil {
		return nil, err
	}
	res := _CallableObject{
		delegate: fn,
		props:    &starlark.Dict{},
	}
	return &res, nil
}

type CallableObject interface {
	starlark.Callable
	starlark.HasSetField
}

// _CallableObject is a Callable that supports custom attributes. This functionality is akin to Python functions,
// which can also be invoked and can store various custom attributes. For practical examples of how to utilize
// _CallableObject, please refer to integration_test/testdata/callable_object_test.star.
type _CallableObject struct {
	delegate starlark.Callable
	props    *starlark.Dict
}

var _ CallableObject = (*_CallableObject)(nil)

func (r *_CallableObject) String() string        { return r.delegate.String() }
func (r *_CallableObject) Type() string          { return CallableObjectType }
func (r *_CallableObject) Freeze()               { r.delegate.Freeze(); r.props.Freeze() }
func (r *_CallableObject) Truth() starlark.Bool  { return r.delegate.Truth() }
func (r *_CallableObject) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: callable_object") }

func (r *_CallableObject) Name() string {
	return r.delegate.Name()
}

func (r *_CallableObject) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.delegate.CallInternal(thread, args, kwargs)
}

func (r *_CallableObject) Attr(name string) (starlark.Value, error) {
	v, found, err := r.props.Get(starlark.String(name))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return v, nil
}

func (r *_CallableObject) AttrNames() []string {
	res := make([]string, r.props.Len())
	for i, key := range r.props.Keys() {
		res[i] = key.(starlark.String).GoString()
	}
	return res
}

func (r *_CallableObject) SetField(name string, value starlark.Value) error {
	return r.props.SetKey(starlark.String(name), value)
}
