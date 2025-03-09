package cad

import (
	"fmt"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type Module struct {
	info *workflow.Info
}

var _ starlark.HasAttrs = &Module{}

func (r *Module) String() string                        { return "temporal" }
func (r *Module) Type() string                          { return "temporal" }
func (r *Module) Freeze()                               {}
func (r *Module) Truth() starlark.Bool                  { return true }
func (r *Module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *Module) Attr(n string) (starlark.Value, error) { return star.Attr(r, n, builtins, properties) }
func (r *Module) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var builtins = map[string]*starlark.Builtin{
	"execute_activity": starlark.NewBuiltin("execute_activity", _executeActivity),
	"execute_workflow": starlark.NewBuiltin("execute_workflow", _executeWorkflow),
}

var properties = map[string]star.PropertyFactory{
	"execution_id":     _executionID,
	"execution_run_id": _executionRunID,
}

func _executionID(receiver starlark.Value) (starlark.Value, error) {
	info := receiver.(*Module).info
	return starlark.String(info.WorkflowExecution.ID), nil
}

func _executionRunID(receiver starlark.Value) (starlark.Value, error) {
	info := receiver.(*Module).info
	return starlark.String(info.WorkflowExecution.RunID), nil
}

func _executeActivity(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	activityID := args[0].(starlark.String).GoString()
	activityArgs := sliceTuple(args[1:])
	var ctx = cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var asBytes bool
	var taskQueue string
	for _, kv := range kwargs {
		k := kv[0].(starlark.String)
		switch k {
		case "task_queue":
			taskQueue = kv[1].(starlark.String).GoString()
		case "as_bytes":
			asBytes = bool(kv[1].(starlark.Bool))
		case "headers":
			// TODO: Implement context propagator for headers
			err := temporal.NewApplicationError("unimplemented", "Feature not yet available")
			logger.Error("builtin-error", "error", err)
			return nil, err
		default:
			err := temporal.NewApplicationError("invalid_argument", fmt.Sprintf("unsupported key: %v", k))
			logger.Error("builtin-error", "error", err)
			return nil, err
		}
	}

	// Ensure task queue is set
	if taskQueue == "" {
		err := temporal.NewApplicationError("invalid_argument", "task_queue must be specified")
		logger.Error("builtin-error", "error", err)
		return nil, err
	}

	// Execute Activity
	ao := workflow.ActivityOptions{
		TaskQueue: taskQueue,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	f := workflow.ExecuteActivity(ctx, activityID, activityArgs...)
	return executeFuture(ctx, f, asBytes)
}

func _executeWorkflow(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	workflowID := args[0].(starlark.String).GoString()
	workflowArgs := sliceTuple(args[1:])
	var ctx = cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var asBytes bool
	var taskQueue string
	for _, kv := range kwargs {
		k := kv[0].(starlark.String)
		switch k {
		case "task_queue":
			taskQueue = kv[1].(starlark.String).GoString()
		case "as_bytes":
			asBytes = bool(kv[1].(starlark.Bool))
		case "headers":
			// TODO: Implement context propagator for headers
			err := temporal.NewApplicationError("unimplemented", "Feature not yet available")
			logger.Error("builtin-error", "error", err)
			return nil, err
		default:
			err := temporal.NewApplicationError("invalid_argument", fmt.Sprintf("unsupported key: %v", k))
			logger.Error("builtin-error", "error", err)
			return nil, err
		}
	}

	// Ensure task queue is set
	if taskQueue == "" {
		err := temporal.NewApplicationError("invalid_argument", "task_queue must be specified")
		logger.Error("builtin-error", "error", err)
		return nil, err
	}

	// Execute Child Workflow
	// Execute Child Workflow
	cwo := workflow.ChildWorkflowOptions{
		ParentClosePolicy:        enumspb.PARENT_CLOSE_POLICY_TERMINATE,
		WorkflowExecutionTimeout: 24 * time.Hour,   // 24 hours execution timeout
		WorkflowRunTimeout:       1 * time.Hour,    // Each run has 1-hour timeout
		WorkflowTaskTimeout:      10 * time.Second, // Each task has 10s timeout
		TaskQueue:                taskQueue,
	}

	// Execute the child workflow with options
	f := workflow.ExecuteChildWorkflow(ctx, workflowID, workflowArgs, cwo)

	return executeFuture(ctx, f, asBytes)
}

func executeFuture(
	ctx workflow.Context,
	future workflow.Future,
	asBytes bool,
) (starlark.Value, error) {
	var err error
	var resBytes []byte
	var resValue starlark.Value

	if asBytes {
		err = future.Get(ctx, &resBytes)
	} else {
		err = future.Get(ctx, &resValue)
	}
	if err != nil {
		workflow.GetLogger(ctx).Error("builtin-error", "asBytes", asBytes, "error", err)
		return nil, err
	}
	if asBytes {
		return starlark.Bytes(resBytes), nil
	} else {
		return resValue, nil
	}
}

func sliceTuple(args starlark.Tuple) []any {
	res := make([]any, args.Len())
	star.Iterate(args, func(i int, el starlark.Value) {
		res[i] = el
	})
	return res
}
