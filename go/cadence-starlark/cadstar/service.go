package cadstar

import (
	"container/list"
	"errors"
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/ext"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

type contextKey int

const contextKeyGlobals contextKey = iota

var builtins = starlark.StringDict{
	star.CallableObjectType: star.CallableObjectBuiltin,
}

type Meta struct {
	MainFile     string `json:"main_file,omitempty"`
	MainFunction string `json:"main_function,omitempty"`
}

type _Globals struct {
	exitHooks  *ExitHooks
	isCanceled bool
	logs       *list.List
	environ    *starlark.Dict
	progress   *list.List
}

func (r *_Globals) getEnviron(key starlark.String) (starlark.String, bool) {
	v, found, err := r.environ.Get(key)
	if err != nil {
		panic(err)
	}
	if !found {
		return "", false
	}
	return v.(starlark.String), true
}

type Service struct {
	Plugins        []IPlugin
	ClientTaskList string
}

func (r *Service) Register(registry worker.Registry) {
	registry.RegisterWorkflow(r.Run)
}
func (r *Service) Run(
	ctx workflow.Context,
	tar []byte,
	path string,
	function string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	environ *starlark.Dict,
) (
	res starlark.Value,
	err error,
) {
	logger := workflow.GetLogger(ctx)

	defer func() {
		if rec := recover(); rec != nil {
			logger.Error("workflow-panic", zap.Any("panic", rec))
			err = temporal.NewApplicationError(
				fmt.Sprintf("panic: %v", rec),
				"panic_error",
			)
		}
	}()

	logger.Info(
		"workflow-start",
		zap.String("path", path),
		zap.String("function", function),
		zap.Int("tar_len", len(tar)),
	)

	if environ == nil {
		environ = &starlark.Dict{}
	}

	// Fix: Set ActivityOptions correctly
	ao := ext.TemporalDefaultActivityOptions
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Fix: Correctly setting ChildWorkflowOptions
	cwo := workflow.ChildWorkflowOptions{
		WorkflowExecutionTimeout: time.Hour * 24 * 365 * 10, // 10 years
		RetryPolicy:              nil,                       // No retry for child workflows
	}
	ctx = workflow.WithChildOptions(ctx, cwo)

	globals := &_Globals{
		exitHooks:  &ExitHooks{},
		isCanceled: false,
		logs:       list.New(),
		environ:    environ,
		progress:   list.New(),
	}
	ctx = workflow.WithValue(ctx, contextKeyGlobals, globals)

	var fs star.FS
	if fs, err = star.TarFS(tar); err != nil {
		logger.Error("workflow-error", zap.Error(err))
		return nil, temporal.NewApplicationError(
			err.Error(),
			"invalid_argument",
		)
	}

	meta := Meta{}
	if b, readErr := fs.Read("/meta.json"); readErr != nil {
		if !errors.Is(readErr, star.ErrNotExist) {
			return nil, readErr
		}
	} else {
		if err = jsoniter.Unmarshal(b, &meta); err != nil {
			return nil, err
		}
	}
	logger.Info("workflow-meta", zap.Any("meta", meta))

	if path == "" {
		path = meta.MainFile
	}
	if function == "" {
		function = meta.MainFunction
	}

	runInfo := RunInfo{
		Info:    workflow.GetInfo(ctx),
		Environ: environ,
	}

	plugin := starlark.StringDict{}
	for _, p := range r.Plugins {
		for k, v := range p.Create(runInfo) {
			plugin[k] = v
		}
	}

	if err = workflow.SetQueryHandler(ctx, "logs", func() (any, error) {
		logs := make([]any, globals.logs.Len())
		var i int
		for e := globals.logs.Front(); e != nil; e = e.Next() {
			logs[i] = e.Value
			i++
		}
		return logs, nil
	}); err != nil {
		logger.Error("workflow-error", zap.Error(err))
		return nil, err
	}

	if err = workflow.SetQueryHandler(ctx, "task_progress", func() (any, error) {
		progress := make([]any, globals.progress.Len())
		var i int
		for e := globals.progress.Front(); e != nil; e = e.Next() {
			progress[i] = e.Value
			i++
		}
		return progress, nil
	}); err != nil {
		logger.Error("workflow-error", zap.Error(err))
		return nil, err
	}

	t := CreateThread(ctx)
	t.Load = star.ThreadLoad(fs, builtins, map[string]starlark.StringDict{"plugin": plugin})

	// Run main user code
	if res, err = star.Call(t, path, function, args, kwargs); err != nil {
		logger.Error("workflow-error", zap.Error(err))

		if errors.Is(err, workflow.ErrCanceled) {
			globals.isCanceled = true
			ctx, _ = workflow.NewDisconnectedContext(ctx)
		}
	}

	// Run exit hooks
	if _err := globals.exitHooks.Run(t); _err != nil {
		logger.Error("exit-hook-error", zap.Error(_err))
		err = errors.Join(err, _err)
	}

	err = processError(ctx, err)

	logger.Info("workflow-end")
	return res, err
}

func processError(ctx workflow.Context, err error) error {
	if err == nil {
		return nil
	}
	logger := workflow.GetLogger(ctx)

	details := map[string]any{"error": err.Error()}
	var evalErr *starlark.EvalError
	if errors.As(err, &evalErr) {
		logger.Error("starlark-backtrace", zap.String("backtrace", evalErr.Backtrace()))
		details["backtrace"] = evalErr.Backtrace()
	}
	return temporal.NewApplicationError("execution_failed", "starlark_execution_error", details)
}

func GetExitHooks(ctx workflow.Context) *ExitHooks {
	return getGlobals(ctx).exitHooks
}

func getGlobals(ctx workflow.Context) *_Globals {
	return ctx.Value(contextKeyGlobals).(*_Globals)
}

func GetProgress(ctx workflow.Context) *list.List {
	return getGlobals(ctx).progress
}
