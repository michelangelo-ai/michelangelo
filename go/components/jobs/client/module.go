package client

import (
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"go.uber.org/fx"
)

// Module provides client for jobs
// related operations
var Module = fx.Options(
	fx.Provide(NewClient),
	fx.Provide(k8sengine.NewMapper),
	fx.Provide(NewHelper),
	compute.Module,
	secrets.Module,
)
