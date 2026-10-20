package job

import (
	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobsCluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
)

// Module FX
var Module = fx.Options(
	fx.Invoke(register),
)

// params collects the Reconciler's dependencies. It is an fx.In struct because
// the k8sengine Mapper is provided under a name, which plain positional
// parameters cannot express.
type params struct {
	fx.In

	Conf            cluster.Config
	Logger          logr.Logger
	Env             env.Context
	Manager         manager.Manager
	FederatedClient jobsclient.FederatedClient
	ClusterCache    jobsCluster.RegisteredClustersCache
	Mapper          matypes.Mapper `name:"k8sengineMapper"`
	MetricsScope    tally.Scope
}

func register(p params) error {
	restConfig := p.Manager.GetConfig()
	restConfig.QPS = p.Conf.QPS
	restConfig.Burst = p.Conf.Burst

	return NewReconciler(
		p.Logger,
		p.Manager.GetClient(),
		p.Env,
		p.FederatedClient,
		p.ClusterCache,
		p.Mapper,
		p.MetricsScope,
	).Register(p.Manager)
}
