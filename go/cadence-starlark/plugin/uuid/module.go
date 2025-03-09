package uuid

import (
	"fmt"
	_uuid "github.com/google/uuid"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

type Module struct{}

var _ starlark.HasAttrs = &Module{}

func (f *Module) String() string                        { return "uuid" }
func (f *Module) Type() string                          { return "uuid" }
func (f *Module) Freeze()                               {}
func (f *Module) Truth() starlark.Bool                  { return true }
func (f *Module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *Module) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *Module) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var builtins = map[string]*starlark.Builtin{
	"uuid4": starlark.NewBuiltin("uuid4", uuid4),
}

var properties = map[string]star.PropertyFactory{}

func uuid4(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx) // ✅ Temporal uses `workflow.Logger(ctx)`

	if err := starlark.UnpackArgs("uuid4", args, kwargs); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	// ✅ Temporal requires JSON serialization for SideEffect
	_stringUUID := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return _uuid.New().String()
	})

	var stringUUID string
	if err := _stringUUID.Get(&stringUUID); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	return &UUID{StringUUID: starlark.String(stringUUID)}, nil
}
