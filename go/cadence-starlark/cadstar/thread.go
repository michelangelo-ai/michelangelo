package cadstar

import (
	"strconv"

	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"
)

const threadLocalContextKey string = "context"
const envLogLen = starlark.String("STAR_CORE_LOG_LEN")
const defaultLogLen = 1000

func CreateThread(ctx workflow.Context) *starlark.Thread {
	globals := getGlobals(ctx)

	ll := defaultLogLen
	if v, found := globals.getEnviron(envLogLen); found {
		var err error
		if ll, err = strconv.Atoi(v.GoString()); err != nil {
			ll = defaultLogLen
		}
	}

	logs := globals.logs
	t := &starlark.Thread{
		Print: func(t *starlark.Thread, msg string) {
			logs.PushBack(msg)
			if logs.Len() > ll {
				logs.Remove(logs.Front())
			}
		},
	}
	t.SetLocal(threadLocalContextKey, ctx)
	return t
}

func GetContext(t *starlark.Thread) workflow.Context {
	ctx := t.Local(threadLocalContextKey).(workflow.Context)
	if ctx == nil {
		panic("workflow context is missing in thread local storage")
	}

	// ✅ Temporal does NOT support `NewDisconnectedContext`
	// Instead, return the context as is, since cancelation is handled differently
	if getGlobals(ctx).isCanceled {
		ctx, _ = workflow.NewDisconnectedContext(ctx)
		println("Attempted to use a canceled workflow context")
	}
	return ctx
}
