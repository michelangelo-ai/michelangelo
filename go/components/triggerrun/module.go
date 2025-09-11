package triggerrun

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
)

// Module provides fx Options for triggerrun controller.
var Module = fx.Options(
	fx.Invoke(register),
)

func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	workflowClient clientInterface.WorkflowClient,
) error {
	cronTrigger := NewCronTrigger(
		mgr.GetLogger().WithName("cron-trigger"),
		workflowClient,
	)
	reconciler := NewReconciler(Params{
		Logger:            mgr.GetLogger().WithName("triggerrun"),
		WorkflowClient:    workflowClient,
		APIHandlerFactory: apiHandlerFactory,
		CronTrigger:       cronTrigger,
		// TODO: Add other trigger types as needed
		BackfillTrigger:   cronTrigger, // placeholder
		BatchRerunTrigger: cronTrigger, // placeholder
	})
	return reconciler.Register(mgr)
}
