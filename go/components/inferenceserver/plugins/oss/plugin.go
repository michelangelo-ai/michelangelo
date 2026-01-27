package oss

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Module = fx.Options(
	fx.Invoke(registerPlugins),
)

// registerPlugins registers all OSS plugins with the plugin registry
func registerPlugins(registry plugins.PluginRegistry, clientFactory clientfactory.ClientFactory, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry, config baseconfig.InferenceServerConfig, recorder record.EventRecorder, logger *zap.Logger) {
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, triton.NewPlugin(backends.NewTritonBackend(clientFactory, modelConfigMapProvider, logger), endpointRegistry, modelConfigMapProvider, config.ControlPlaneClusterId, recorder, logger))
}
