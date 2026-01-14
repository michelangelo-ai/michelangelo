package oss

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RegisterPlugins registers all OSS plugins with the plugin registry
func RegisterPlugins(registry plugins.PluginRegistry, kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry, recorder record.EventRecorder, logger *zap.Logger) {
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, triton.NewPlugin(backends.NewTritonBackend(kubeClient, modelConfigMapProvider, endpointRegistry, logger), modelConfigMapProvider, recorder, logger))
}
