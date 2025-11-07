package oss

import (
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/triton"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RegisterPlugins registers all OSS plugins with the plugin registry
func RegisterPlugins(registry plugins.PluginRegistry, gateway gateways.Gateway) {
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, triton.NewPlugin(gateway))
}
