package compute

import (
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
	"go.uber.org/fx"
)

// Module provides the compute client set
var Module = fx.Module("compute",
	kuberay.Module,
	fx.Provide(NewClientSetFactory),
)
