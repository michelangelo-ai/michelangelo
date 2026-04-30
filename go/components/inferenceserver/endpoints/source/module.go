package source

import (
	"go.uber.org/fx"
	"go.uber.org/zap"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpoints"
)

// Module binds the k3d EndpointSource into the fx graph. Include this module
// alongside endpoints.Module when running against k3d clusters.
var Module = fx.Options(
	fx.Provide(newK3dSource),
)

func newK3dSource(clientFactory clientfactory.ClientFactory, isConfig maconfig.InferenceServerConfig, logger *zap.Logger) endpoints.EndpointSource {
	return NewK3dSource(clientFactory, isConfig, logger)
}
