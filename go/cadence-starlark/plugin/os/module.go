package os

import (
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
)

type Module struct {
	environ *starlark.Dict
}

var _ starlark.HasAttrs = &Module{}

func (f *Module) String() string                        { return "os" }
func (f *Module) Type() string                          { return "os" }
func (f *Module) Freeze()                               {}
func (f *Module) Truth() starlark.Bool                  { return true }
func (f *Module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *Module) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *Module) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var builtins = map[string]*starlark.Builtin{}

var properties = map[string]star.PropertyFactory{
	"environ": environ,
}

func environ(receiver starlark.Value) (starlark.Value, error) {
	return receiver.(*Module).environ, nil
}
