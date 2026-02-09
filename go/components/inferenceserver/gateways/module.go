package gateways

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
)

var Module = fx.Options(
	fx.Provide(newInferenceServerGateway),
)

// newInferenceServerGateway creates a new inference server gateway using the shared backend registry.
func newInferenceServerGateway(registry *backends.Registry) Gateway {
	return NewGatewayWithBackends(Params{
		Registry: registry,
	})
}
