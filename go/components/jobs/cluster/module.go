package cluster

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.uber.org/zap"

	"github.com/uber-go/tally"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
)

// Module provides the cluster reconciler and scheduler.
var Module = fx.Options(
	fx.Invoke(register),
	fx.Provide(NewReconciler),
)

func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	clusterClient client.FederatedClient,
	scope tally.Scope,
) error {
	return NewReconciler(Params{
		ClusterClient:     clusterClient,
		Scope:             scope,
		APIHandlerFactory: apiHandlerFactory,
	}).Reconciler.SetupWithManager(mgr)
}
