package inferenceserver

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Options(
	endpointregistry.Module,
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
func newPluginRegistry(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry, recorder record.EventRecorder, logger *zap.Logger) plugins.PluginRegistry {
	registry := plugins.NewPluginRegistry()
	oss.RegisterPlugins(registry, kubeClient, modelConfigMapProvider, endpointRegistry, recorder, logger)
	return registry
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
