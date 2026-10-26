package cluster

import (
	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

// params collects the Reconciler's dependencies. It is an fx.In struct because
// the k8sengine Mapper is provided under a name, which plain positional
// parameters cannot express.
type params struct {
	fx.In

	Logger            logr.Logger
	APIHandlerFactory apiHandler.Factory
	Env               env.Context
	Manager           manager.Manager
	SchedulerQueue    scheduler.JobQueue
	FederatedClient   client.FederatedClient
	ClusterCache      cluster.RegisteredClustersCache
	Mapper            matypes.Mapper `name:"k8sengineMapper"`
	MetricsScope      tally.Scope
}

func register(p params) error {
	return NewReconciler(
		p.Logger,
		p.APIHandlerFactory,
		p.Env,
		p.SchedulerQueue,
		p.FederatedClient,
		p.ClusterCache,
		p.Mapper,
		p.MetricsScope,
	).Register(p.Manager)
}
