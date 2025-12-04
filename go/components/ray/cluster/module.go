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

// Module provides Uber FX dependency injection options for the RayCluster controller.
//
// This module registers the RayCluster controller with the Kubernetes controller manager
// and provides configuration loading via the newConfig provider.
//
// Dependencies:
//   - Config: Loaded from configuration provider
//   - JobQueue: For scheduling clusters
//   - FederatedClient: For managing clusters on remote Kubernetes
//   - RegisteredClustersCache: For looking up available clusters
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

// register initializes and registers the RayCluster controller with the manager.
//
// This function is invoked by Uber FX during application startup. It constructs
// the Reconciler with all required dependencies and registers it with the
// Kubernetes controller manager.
//
// The controller will begin watching RayCluster resources and processing them
// through their lifecycle once the manager starts.
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
