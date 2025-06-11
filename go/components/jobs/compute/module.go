package compute

import (
	infraAuth "code.uber.internal/infra/compute/k8s-auth"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray/kuberay"
	"go.uber.org/fx"
)

// Module provides the compute client set
var Module = fx.Module("compute",
	infraAuth.ClientAuthMapModule,
	kuberay.Module,
	fx.Provide(NewClientSetFactory),
)
