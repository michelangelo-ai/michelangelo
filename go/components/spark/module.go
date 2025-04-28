package spark

import (
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job/client"
	"go.uber.org/fx"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Invoke(client.Module),
		fx.Invoke(job.Module),
	)
)
