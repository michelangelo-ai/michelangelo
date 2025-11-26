package oss

import (
	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RegisterPlugins registers all OSS plugins with the plugin registry
func RegisterPlugins(registry plugins.PluginRegistry, gateway gateways.Gateway, modelConfigMapProvider configmap.ModelConfigMapProvider, proxyProvider proxy.ProxyProvider, recorder record.EventRecorder, logger *zap.Logger) {
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, triton.NewPlugin(gateway, modelConfigMapProvider, proxyProvider, recorder, logger))
}
