package triggerrun

import (
	uberconfig "go.uber.org/config"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
)

// Module provides Uber FX dependency injection options for the TriggerRun controller.
//
// This module registers the controller with the Kubernetes controller manager and
// initializes Runner implementations for supported trigger types (cron and backfill).
//
// Usage:
//
//	fx.New(
//	    triggerrun.Module,
//	    // other modules...
//	)
var Module = fx.Options(
	fx.Invoke(register),
)

// register initializes and registers the TriggerRun controller with the manager.
//
// This function is invoked by Uber FX during application startup. It creates Runner
// implementations for cron and backfill triggers, constructs the reconciler with these
// runners, and registers the controller with the Kubernetes controller manager.
//
// Currently supports:
//   - CronTrigger: Recurring workflows based on cron expressions
//   - BackfillTrigger: One-time workflows for historical data processing
//   - BatchRerunTrigger: One-time workflows that resume a batch of prior pipeline runs
//
// Interval triggers are planned but not yet implemented. See TODO(#548).
func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	workflowClient clientInterface.WorkflowClient,
	configProvider uberconfig.Provider,
) error {
	cronTrigger := NewCronTrigger(
		mgr.GetLogger().WithName("cron-trigger"),
		workflowClient,
		configProvider,
	)
	backfillTrigger := NewBackfillTrigger(
		mgr.GetLogger().WithName("backfill-trigger"),
		workflowClient,
		configProvider,
	)
	batchRerunTrigger := NewBatchRerunTrigger(
		mgr.GetLogger().WithName("batch-rerun-trigger"),
		workflowClient,
		configProvider,
	)
	reconciler := NewReconciler(Params{
		Logger:            mgr.GetLogger().WithName("triggerrun"),
		WorkflowClient:    workflowClient,
		APIHandlerFactory: apiHandlerFactory,
		CronTrigger:       cronTrigger,
		BackfillTrigger:   backfillTrigger,
		BatchRerunTrigger: batchRerunTrigger,
		// TODO(#548): Add interval trigger when it's implemented
	})
	return reconciler.Register(mgr)
}
