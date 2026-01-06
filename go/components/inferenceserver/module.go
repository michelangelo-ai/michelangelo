package inferenceserver

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Options(
	configmap.Module,
	gateways.Module,
	fx.Provide(newEventRecorder),
	fx.Provide(newPluginRegistry),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// newEventRecorder creates a new event recorder
func newEventRecorder(mgr ctrl.Manager) record.EventRecorder {
	return mgr.GetEventRecorderFor(ControllerName)
}

// newPluginRegistry creates a new plugin registry with all OSS plugins registered
func newPluginRegistry(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.PluginRegistry {
	return plugins.NewPluginRegistry()
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
