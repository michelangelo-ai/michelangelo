package uuid

import (
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"strings"
)

type UUID struct {
	StringUUID starlark.String
}

var _ starlark.HasAttrs = (*UUID)(nil)

func (r *UUID) String() string                        { return string(r.StringUUID) }
func (r *UUID) Type() string                          { return "UUID" }
func (r *UUID) Freeze()                               { r.StringUUID.Freeze() }
func (r *UUID) Truth() starlark.Bool                  { return r.StringUUID.Truth() }
func (r *UUID) Hash() (uint32, error)                 { return r.StringUUID.Hash() }
func (r *UUID) Attr(n string) (starlark.Value, error) { return star.Attr(r, n, resB, resP) }
func (r *UUID) AttrNames() []string                   { return star.AttrNames(resB, resP) }

var resP = map[string]star.PropertyFactory{
	"urn": _urn,
	"hex": _hex,
}

var resB = map[string]*starlark.Builtin{}

func _urn(r starlark.Value) (starlark.Value, error) {
	return "urn:uuid:" + r.(*UUID).StringUUID, nil
}

func _hex(r starlark.Value) (starlark.Value, error) {
	v := string(r.(*UUID).StringUUID)
	v = strings.ReplaceAll(v, "-", "")
	return starlark.String(v), nil
}
