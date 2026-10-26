package state

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/controllerutil"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Module provides the Kueue state service: the KueueWatches the cluster
// controller drives, and the State and Demand the Kueue scheduler backend
// reads.
var Module = fx.Options(
	fx.Provide(provide),
	fx.Invoke(register),
)

type params struct {
	fx.In

	Config  maconfig.SchedulerConfig
	Factory compute.Factory
	Manager manager.Manager
	Mapper  types.Mapper `name:"k8sengineMapper"`
	Logger  logr.Logger
}

type result struct {
	fx.Out

	Service      *Service
	KueueWatches cluster.KueueWatches
	State        State
	Demand       Demand
}

func provide(p params) (result, error) {
	s, err := newService(p.Config, p.Factory, p.Manager.GetCache(), p.Mapper, p.Logger.WithName("kueue-state"))
	if err != nil {
		return result{}, err
	}
	return result{Service: s, KueueWatches: s, State: s, Demand: s}, nil
}

// register indexes the manager cache's RayClusters by assigned cluster and has
// the manager run the service, under the Kueue backend only.
func register(mgr manager.Manager, s *Service) error {
	if !s.enabled {
		return nil
	}
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &v2pb.RayCluster{}, AssignedClusterIndex, assignedCluster); err != nil {
		return fmt.Errorf("index RayClusters by assigned cluster: %w", err)
	}
	// every replica runs it, so a new leader's cluster controller can start
	// watches under the manager's context right away
	return mgr.Add(controllerutil.NonLeaderRunnable(s.run))
}
