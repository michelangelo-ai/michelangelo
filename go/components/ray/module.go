package ray

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/job"
)

var (
	// Module exports both the cluster and job module
	Module = fx.Options(
		cluster.Module,
		job.Module,
	)
)
