package trigger

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker, workflow workflow.Workflow) {
	ws := workflows{workflow: workflow}
	for _, w := range workers {
		w.RegisterWorkflow(ws.CronTrigger, "trigger.CronTrigger")
	}
}
