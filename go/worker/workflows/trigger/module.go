package trigger

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker) {
	ws := &workflows{}
	for _, w := range workers {
		w.RegisterWorkflow(ws.CronTrigger, "trigger.CronTrigger")
	}
}
