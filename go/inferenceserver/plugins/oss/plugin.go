package oss

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins/oss/triton"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins/oss/llmd"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins/oss/dynamo"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RegisterPlugins registers all OSS plugins with the plugin registry
func RegisterPlugins(registry plugins.PluginRegistry, gateway inferenceserver.Gateway) {
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, triton.NewPlugin(gateway))
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_LLM_D, llmd.NewPlugin(gateway))
	registry.RegisterPlugin(v2pb.BACKEND_TYPE_DYNAMO, dynamo.NewPlugin(gateway))
}