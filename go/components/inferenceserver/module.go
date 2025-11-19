package inferenceserver

import (
	"fmt"

	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	triton "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	configKey = "gateways"
)

// GatewayConfig defines configuration for gateway backends
type GatewayConfig struct {
	// InferenceServiceEndpoint is the base URL for reaching inference services
	// e.g., "http://localhost:8889" for local development
	// or "http://istio-gateway.default.svc.cluster.local:80" for in-cluster
	InferenceServiceEndpoint string `yaml:"inferenceServiceEndpoint"`
}

// Module provides the inference server controller with all dependencies
var Module = fx.Module("inferenceserver",
	fx.Provide(NewGatewayConfig),
	fx.Provide(NewInferenceServerGateway),
	fx.Provide(NewDynamicClient),
	fx.Provide(proxy.NewHTTPRouteManager),
	fx.Provide(NewPluginRegistry),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewGatewayConfig creates a new gateway configuration from the config provider
func NewGatewayConfig(provider config.Provider) (GatewayConfig, error) {
	var conf GatewayConfig
	// Set default value if not configured
	conf.InferenceServiceEndpoint = "http://localhost:8889"

	// Try to populate from config, but don't fail if the key doesn't exist
	if err := provider.Get(configKey).Populate(&conf); err != nil {
		// Config key doesn't exist, use default
		return conf, nil
	}
	return conf, nil
}

func NewDynamicClient(restConfig *rest.Config) (dynamic.Interface, error) {
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

// NewInferenceServerGateway creates a new inference server gateway with clients
func NewInferenceServerGateway(kubeClient client.Client, dynamicClient dynamic.Interface, gatewayConfig GatewayConfig, logger *zap.Logger) gateways.Gateway {
	// // Create dynamic client from the same config as kube client
	// restConfig, err := ctrl.GetConfig()
	// if err != nil {
	// 	panic(fmt.Errorf("failed to get REST config: %w", err))
	// }

	// dynamicClient, err := dynamic.NewForConfig(restConfig)
	// if err != nil {
	// 	panic(fmt.Errorf("failed to create dynamic client: %w", err))
	// }

	gateway := gateways.NewGatewayWithClients(gateways.Params{
		KubeClient:             kubeClient,
		DynamicClient:          dynamicClient,
		ModelConfigMapProvider: configmap.NewDefaultModelConfigMapProvider(kubeClient, logger),
	})

	// Register Triton backend with its endpoint configuration
	gateway.RegisterBackend(
		v2pb.BACKEND_TYPE_TRITON,
		triton.NewTritonBackend(
			kubeClient,
			dynamicClient,
			configmap.NewDefaultModelConfigMapProvider(kubeClient, logger),
			gatewayConfig.InferenceServiceEndpoint,
			logger,
		),
	)

	return gateway
}

// NewPluginRegistry creates a new plugin registry with all OSS plugins registered
func NewPluginRegistry(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider) plugins.PluginRegistry {
	registry := plugins.NewPluginRegistry()
	oss.RegisterPlugins(registry, gateway, proxyProvider)
	return registry
}

// NewReconciler creates a new inference server reconciler
func NewReconciler(mgr ctrl.Manager, scheme *runtime.Scheme, gateway gateways.Gateway, proxyProvider proxy.ProxyProvider, pluginRegistry plugins.PluginRegistry, logger *zap.Logger) *Reconciler {
	logger = logger.With(zap.String("component", "inferenceserver"))
	return &Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        scheme,
		Recorder:      mgr.GetEventRecorderFor(ControllerName),
		Gateway:       gateway,
		Plugins:       pluginRegistry,
		ProxyProvider: proxyProvider,
		logger:        logger,
	}
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
