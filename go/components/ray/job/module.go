package job

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobsCluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
)

// Module provides Uber FX dependency injection options for the RayJob controller.
//
// This module registers the RayJob controller with the Kubernetes controller manager.
// The controller manages Ray jobs that execute on Ray clusters, handling job submission,
// status monitoring, and dependency management with RayCluster resources.
//
// Dependencies:
//   - Config: Loaded from RayCluster controller configuration
//   - FederatedClient: For creating and monitoring jobs on remote Kubernetes clusters
//   - RegisteredClustersCache: For looking up available physical clusters
var Module = fx.Options(
	fx.Invoke(register),
)

// register initializes and registers the RayJob controller with the manager.
//
// This function is invoked by Uber FX during application startup. It constructs
// the Reconciler with all required dependencies and registers it with the
// Kubernetes controller manager.
//
// The controller inherits QPS and Burst rate limiting settings from the RayCluster
// controller configuration to ensure consistent API server interaction patterns.
//
// The controller will begin watching RayJob resources and processing them
// through their lifecycle once the manager starts.
func register(
	conf cluster.Config,
	env env.Context,
	mgr manager.Manager,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobsCluster.RegisteredClustersCache,
) error {
	restConfig := mgr.GetConfig()
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst

	return (&Reconciler{
		Client:          mgr.GetClient(),
		env:             env,
		federatedClient: federatedClient,
		clusterCache:    clusterCache,
	}).Register(mgr)
}
