package inferenceserver

import (
	"go.uber.org/fx"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Options(
	gateways.Module,
	fx.Provide(newEventRecorder),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// newEventRecorder creates a new event recorder
func newEventRecorder(mgr ctrl.Manager) record.EventRecorder {
	return mgr.GetEventRecorderFor(ControllerName)
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
