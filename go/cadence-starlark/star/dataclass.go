package star

import (
	"encoding/json"
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type Dataclass struct {
	Dict *starlark.Dict
}

var _ starlark.Comparable = (*Dataclass)(nil)
var _ starlark.HasSetField = (*Dataclass)(nil)
var _ json.Marshaler = (*Dataclass)(nil)

func (r *Dataclass) String() string        { return r.Dict.String() }
func (r *Dataclass) Type() string          { return r.Dict.Type() }
func (r *Dataclass) Freeze()               { r.Dict.Freeze() }
func (r *Dataclass) Truth() starlark.Bool  { return r.Dict.Truth() }
func (r *Dataclass) Hash() (uint32, error) { return r.Dict.Hash() }

func (r *Dataclass) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*Dataclass).Dict
	return r.Dict.CompareSameType(op, y, depth)
}

func (r *Dataclass) Attr(name string) (starlark.Value, error) {
	if name == "__dict__" {
		return r.Dict, nil
	}
	v, found, err := r.Dict.Get(starlark.String(name))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return v, nil
}

func (r *Dataclass) AttrNames() []string {
	attrs := make([]string, r.Dict.Len())
	for i, key := range r.Dict.Keys() {
		attrs[i] = string(key.(starlark.String))
	}
	return attrs
}

func (r *Dataclass) SetField(name string, val starlark.Value) error {
	k := starlark.String(name)
	_, found, err := r.Dict.Get(k)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("attribute not found: %s", name)
	}
	return r.Dict.SetKey(k, val)
}

func (r *Dataclass) MarshalJSON() ([]byte, error) {
	return Encode(r.Dict)
}
