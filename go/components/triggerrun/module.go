package triggerrun

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun/cadence"
)

// Module provides fx Options for triggerrun controller.
var Module = fx.Options(
	cadence.Module,
	fx.Invoke(register),
)

func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	cadenceClient clientInterface.WorkflowClient,
) error {
	cronTrigger := NewCronTrigger(
		mgr.GetLogger().WithName("cron-trigger"),
		cadenceClient,
	)
	reconciler := NewReconciler(Params{
		Logger:            mgr.GetLogger().WithName("triggerrun"),
		CadenceClient:     cadenceClient,
		APIHandlerFactory: apiHandlerFactory,
		CronTrigger:       cronTrigger,
		// TODO: Add other trigger types as needed
		BackfillTrigger:   cronTrigger, // placeholder
		BatchRerunTrigger: cronTrigger, // placeholder
	})
	return reconciler.Register(mgr)
}
