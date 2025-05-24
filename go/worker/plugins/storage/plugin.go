package storage

import (
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
)

const pluginID = "storage"

var Plugin = &plugin{}

type plugin struct{}

var _ service.IPlugin = (*plugin)(nil)

func (r *plugin) ID() string {
	return pluginID
}
func (r *plugin) Create(_ service.RunInfo) starlark.Value {
	return newModule()
}
func (r *plugin) Register(_ worker.Registry) {}

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

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"read": starlark.NewBuiltin("read", m.read).BindReceiver(m),
	}
	return m
}

func (m *module) read(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
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

	var res any
	// Here we not return error to starlark which causing termination of workflow but return error message
	// to leave starlark to handle it
	readErr := workflow.ExecuteActivity(ctx, storage.Activities.Read, protocol, path).Get(ctx, &res)
	var errRet starlark.Value
	if readErr != nil {
		logger.Error("builtin-error", ext.ZapError(readErr)...)
		if err := utils.AsStar(readErr, &errRet); err != nil {
			logger.Error("builtin-error", ext.ZapError(err)...)
			return nil, err
		}
	} else {
		errRet = starlark.None
	}
	var ret starlark.Value
	if err := utils.AsStar(res, &ret); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return starlark.Tuple{ret, errRet}, nil
}
