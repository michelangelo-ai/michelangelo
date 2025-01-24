package ray

import (
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker) {
	ws := &workflows{}
	for _, w := range workers {
		w.RegisterWorkflow(ws.CreateRayCluster)
	}
}
