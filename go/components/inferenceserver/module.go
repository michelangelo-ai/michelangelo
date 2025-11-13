package inferenceserver

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss"
	configmap "github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	triton "github.com/michelangelo-ai/michelangelo/go/shared/gateways/backends"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Module("inferenceserver",
	fx.Provide(NewInferenceServerGateway),
	fx.Provide(NewPluginRegistry),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewInferenceServerGateway creates a new inference server gateway with clients
func NewInferenceServerGateway(kubeClient client.Client, logger *zap.Logger) gateways.Gateway {
	// Create dynamic client from the same config as kube client
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to get REST config: %w", err))
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %w", err))
	}

	gateway := gateways.NewGatewayWithClients(kubeClient, dynamicClient, logger)
	gateway.RegisterBackend(v2pb.BACKEND_TYPE_TRITON, triton.NewTritonBackend(kubeClient, dynamicClient, configmap.NewDefaultConfigMapProvider(kubeClient, logger), logger))

	return gateway
}

// NewPluginRegistry creates a new plugin registry with all OSS plugins registered
func NewPluginRegistry(gateway gateways.Gateway) plugins.PluginRegistry {
	registry := plugins.NewPluginRegistry()
	oss.RegisterPlugins(registry, gateway)
	return registry
}

// NewReconciler creates a new inference server reconciler
func NewReconciler(mgr ctrl.Manager, scheme *runtime.Scheme, gateway gateways.Gateway, pluginRegistry plugins.PluginRegistry, logger *zap.Logger) *Reconciler {
	logger = logger.With(zap.String("component", "inferenceserver"))
	return &Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   scheme,
		Recorder: mgr.GetEventRecorderFor(ControllerName),
		Gateway:  gateway,
		Plugins:  pluginRegistry,
		logger:   logger,
	}
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
