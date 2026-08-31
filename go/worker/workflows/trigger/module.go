package trigger

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(NewConfig),
	fx.Invoke(register),
)

func register(workers []worker.Worker, wf workflow.Workflow, conf Config) {
	ws := workflows{workflow: wf, defaultEnv: conf.DefaultEnvironment}
	for _, w := range workers {
		w.RegisterWorkflow(ws.CronTrigger, "trigger.CronTrigger")
		w.RegisterWorkflow(ws.BackfillTrigger, "trigger.BackfillTrigger")
	}
}
