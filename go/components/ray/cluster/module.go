package cluster

import (
	"github.com/go-logr/logr"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
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
	logger logr.Logger,
	apiHandlerFactory apiHandler.Factory,
	env env.Context,
	mgr manager.Manager,
	schedulerQueue scheduler.JobQueue,
	federatedClient client.FederatedClient,
	clusterCache cluster.RegisteredClustersCache,
) error {
	return NewReconciler(
		conf,
		logger,
		apiHandlerFactory,
		env,
		schedulerQueue,
		federatedClient,
		clusterCache,
	).Register(mgr)
}
