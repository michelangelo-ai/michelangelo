package job

import (
	"github.com/go-logr/logr"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobsCluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
)

// Module FX
var Module = fx.Options(
	fx.Invoke(register),
)

func register(
	conf cluster.Config,
	logger logr.Logger,
	env env.Context,
	mgr manager.Manager,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobsCluster.RegisteredClustersCache,
) error {
	restConfig := mgr.GetConfig()
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst

	return NewReconciler(
		logger,
		mgr.GetClient(),
		env,
		federatedClient,
		clusterCache,
	).Register(mgr)
}
