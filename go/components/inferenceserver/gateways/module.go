package gateways

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Module = fx.Options(
	fx.Provide(newInferenceServerGateway),
)

// newInferenceServerGateway creates a new inference server gateway with a default set of backends
func newInferenceServerGateway() Gateway {
	return NewGatewayWithBackends(Params{
		Backends: map[v2pb.BackendType]backends.Backend{
			v2pb.BACKEND_TYPE_TRITON: backends.NewTritonBackend(),
		},
	})
}
