package cluster

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

func register(
	conf Config,
	env env.Context,
	mgr manager.Manager,
	schedulerQueue scheduler.JobQueue,
	federatedClient client.FederatedClient,
	clusterCache cluster.RegisteredClustersCache,
	handler api.Handler,
) error {
	return (&Reconciler{
		Handler:         handler,
		env:             env,
		schedulerQueue:  schedulerQueue,
		federatedClient: federatedClient,
		clusterCache:    clusterCache,
	}).Register(mgr)
}
