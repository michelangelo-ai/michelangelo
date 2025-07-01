package inferenceserver

import (
	"fmt"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Module("inferenceserver",
	fx.Provide(NewInferenceServerGateway),
	fx.Provide(NewPluginRegistry),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewInferenceServerGateway creates a new inference server gateway with clients
func NewInferenceServerGateway(kubeClient client.Client) inferenceserver.Gateway {
	// Create dynamic client from the same config as kube client
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to get REST config: %w", err))
	}
	
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %w", err))
	}
	
	return inferenceserver.NewGatewayWithClients(kubeClient, dynamicClient)
}

// NewPluginRegistry creates a new plugin registry with all OSS plugins registered
func NewPluginRegistry(gateway inferenceserver.Gateway) plugins.PluginRegistry {
	registry := plugins.NewPluginRegistry()
	oss.RegisterPlugins(registry, gateway)
	return registry
}

// NewReconciler creates a new inference server reconciler
func NewReconciler(mgr ctrl.Manager, scheme *runtime.Scheme, gateway inferenceserver.Gateway, pluginRegistry plugins.PluginRegistry) *Reconciler {
	return &Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   scheme,
		Recorder: mgr.GetEventRecorderFor(ControllerName),
		Gateway:  gateway,
		Plugins:  pluginRegistry,
	}
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}