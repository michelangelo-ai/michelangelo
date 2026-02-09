package oss

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
)

var Module = fx.Options(
	fx.Provide(func() backends.Backend { return backends.NewTritonBackend() }),
	fx.Provide(NewOSSPlugin),
)
