package storage

import (
	"fmt"
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/star"
	jsoniter "github.com/json-iterator/go"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/s3"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/workflow"
)

type module struct {
	attributes map[string]starlark.Value
}

func (m *module) String() string                        { return pluginID }
func (m *module) Type() string                          { return pluginID }
func (m *module) Freeze()                               {}
func (m *module) Truth() starlark.Bool                  { return true }
func (m *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (m *module) Attr(n string) (starlark.Value, error) { return m.attributes[n], nil }
func (m *module) AttrNames() []string                   { return ext.SortedKeys(m.attributes) }
func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return star.Decode(b, out)
}
func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"read": starlark.NewBuiltin("read", m.read).BindReceiver(m),
	}
	return m
}

func (m *module) read(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var protocol string
	var path string
	if err := starlark.UnpackArgs("execute", args, kwargs,
		"protocol", &protocol,
		"path", &path,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	req := s3.ReadRequest{
		Bucket: bucket,
		Path:   path,
	}

	// Call the S3 read activity
	var res any
	if err := workflow.ExecuteActivity(ctx, s3.Activities.Read, req).Get(ctx, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	var ret starlark.Value
	if err := AsStar(res, &ret); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return ret, nil
}
