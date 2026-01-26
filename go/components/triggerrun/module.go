package triggerrun

import (
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
// This function is invoked by Uber FX during application startup. It constructs
// the reconciler with RunnerFactory for provider-aware runner selection and
// registers the controller with the Kubernetes controller manager.
//
// The RunnerFactory automatically selects appropriate runners for each trigger type:
//   - CronSchedule: Uses ScheduleTrigger (Temporal) or CronTrigger (Cadence)
//   - TemporalSchedule: Uses ScheduleTrigger (requires Temporal)
//   - BatchRerun: Uses BackfillTrigger (provider-agnostic)
//   - IntervalSchedule: Not yet implemented
func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	workflowClient clientInterface.WorkflowClient,
) error {
	reconciler := NewReconciler(Params{
		Logger:            mgr.GetLogger().WithName("triggerrun"),
		WorkflowClient:    workflowClient,
		APIHandlerFactory: apiHandlerFactory,
	})
	return reconciler.Register(mgr)
}
