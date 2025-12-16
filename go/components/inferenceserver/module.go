package inferenceserver

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Options(
	fx.Provide(newDynamicClient),
	fx.Provide(newModelConfigMapProvider),
	fx.Provide(newInferenceServerGateway),
	fx.Provide(newEventRecorder),
	fx.Provide(newPluginRegistry),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// newDynamicClient creates a Kubernetes dynamic client for working with unstructured resources
func newDynamicClient() (dynamic.Interface, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to get REST config: %w", err))
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %w", err))
	}
	return dynamicClient, nil
}

// newModelConfigMapProvider creates a new model config map provider
func newModelConfigMapProvider(client client.Client, logger *zap.Logger) configmap.ModelConfigMapProvider {
	return configmap.NewDefaultModelConfigMapProvider(client, logger)
}

// newEventRecorder creates a new event recorder
func newEventRecorder(mgr ctrl.Manager) record.EventRecorder {
	return mgr.GetEventRecorderFor(ControllerName)
}

// newPluginRegistry creates a new plugin registry with all OSS plugins registered
func newPluginRegistry(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, recorder record.EventRecorder, logger *zap.Logger) plugins.PluginRegistry {
	registry := plugins.NewPluginRegistry()
	oss.RegisterPlugins(registry, kubeClient, modelConfigMapProvider, recorder, logger)
	return registry
}

// newInferenceServerGateway creates a new inference server gateway with clients
func newInferenceServerGateway(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) gateways.Gateway {
	gateway := gateways.NewGatewayWithClients(gateways.Params{
		Logger:                 logger,
		KubeClient:             kubeClient,
		ModelConfigMapProvider: modelConfigMapProvider,
	})

	return gateway
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
