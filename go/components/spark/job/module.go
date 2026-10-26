package job

import (
	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
)

var (
	// Module provides Uber FX dependency injection options for the SparkJob controller.
	//
	// This module registers the SparkJob controller with the Kubernetes controller manager.
	// The controller admits Spark jobs to the scheduler and launches them, as
	// SparkApplications, on the member cluster the scheduler assigns.
	//
	// Dependencies:
	//   - Manager: Kubernetes controller manager for resource watching
	//   - Environment: Context for configuration
	//   - Logger: For the federated watch's event handlers
	//   - SchedulerQueue: The queue a new SparkJob is admitted onto
	//   - FederatedClient: Multi-cluster client the launch and the SparkApplication watch are built on
	//   - ClusterCache: The member clusters to launch on and to watch
	//   - Mapper: Projects SparkApplication status back onto the global SparkJob
	//   - MetricsScope: Metrics for the watch
	Module = fx.Options(
		fx.Invoke(register),
	)
)

// params collects the Reconciler's dependencies. It is an fx.In struct because
// the k8sengine Mapper is provided under a name, which plain positional
// parameters cannot express.
type params struct {
	fx.In

	Logger          logr.Logger
	Env             env.Context
	Manager         manager.Manager
	SchedulerQueue  scheduler.JobQueue
	FederatedClient jobsclient.FederatedClient
	ClusterCache    jobscluster.RegisteredClustersCache
	Mapper          matypes.Mapper `name:"k8sengineMapper"`
	MetricsScope    tally.Scope
}

// register initializes and registers the SparkJob controller with the manager.
//
// This function is invoked by Uber FX during application startup. It constructs
// the Reconciler with all required dependencies and registers it with the
// Kubernetes controller manager.
//
// The controller will begin watching SparkJob resources and processing them
// through their lifecycle once the manager starts.
func register(p params) error {
	return NewReconciler(
		p.Logger,
		p.Manager.GetClient(),
		p.Env,
		p.SchedulerQueue,
		p.FederatedClient,
		p.ClusterCache,
		p.Mapper,
		p.MetricsScope,
	).Register(p.Manager)
}
