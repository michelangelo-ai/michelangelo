package scheduler

import (
	"fmt"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	commonscheduler "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
	kueuestate "github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue/state"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// _backendDefault and _backendKueue are the recognized values of
	// jobs.scheduler.backend. Empty means default. Any other value is a
	// startup error so a typo cannot silently fall back to the default
	// admission behavior.
	_backendDefault = "default"
	_backendKueue   = "kueue"
)

// Module provides the JobQueue implementation selected by
// jobs.scheduler.backend.
var Module = fx.Options(
	fx.Provide(NewScheduler),
	fx.Provide(provide),
	commonscheduler.Module,
	framework.Module,
	kueue.Module,
)

// ProvideIn carries the dependencies of the backend selection.
type ProvideIn struct {
	fx.In

	Scheduler          *Scheduler
	Config             maconfig.SchedulerConfig
	Manager            ctrl.Manager
	APIHandlerFactory  apiHandler.Factory
	AssignmentStrategy framework.AssignmentStrategy
	ClusterCache       cluster.RegisteredClustersCache
	LocalQueues        kueue.LocalQueues
	// KueueState and KueueDemand are the Kueue state the kueue_aware
	// strategies place jobs by.
	KueueState  kueuestate.State
	KueueDemand kueuestate.Demand
	Scope       tally.Scope
}

// provide returns the JobQueue selected by jobs.scheduler.backend. The Kueue
// backend replaces the default scheduler's decision path: it makes and
// persists each job's single placement decision at enqueue, validating
// LocalQueue placement first for jobs headed to Kueue-managed clusters, so no
// cluster is selected again after validation. It decides by
// jobs.scheduler.assignmentStrategy, which the config allows to be a Kueue
// one only under this backend. The default backend is unchanged.
func provide(in ProvideIn) (JobQueue, error) {
	switch in.Config.Backend {
	case "", _backendDefault:
		return in.Scheduler, nil
	case _backendKueue:
		handler, err := in.APIHandlerFactory.GetAPIHandler(in.Manager.GetClient())
		if err != nil {
			return nil, fmt.Errorf("kueue scheduler backend: %w", err)
		}
		return kueue.NewKueueJobQueue(kueue.Params{
			Handler:            handler,
			AssignmentStrategy: in.AssignmentStrategy,
			ClusterCache:       in.ClusterCache,
			LocalQueues:        in.LocalQueues,
			Config:             in.Config,
			Scope:              in.Scope,
			Logger:             in.Manager.GetLogger(),
		}, kueueOptions(in)...), nil
	default:
		return nil, fmt.Errorf(
			"unrecognized jobs.scheduler.backend %q (expected %q or %q)",
			in.Config.Backend, _backendDefault, _backendKueue)
	}
}

// kueueOptions returns the Kueue backend's options for
// jobs.scheduler.assignmentStrategy: kueue_aware and kueue_aware_shadow add
// the kueue_aware strategy, and cluster_only, the default, adds nothing.
func kueueOptions(in ProvideIn) []kueue.Option {
	switch in.Config.AssignmentStrategy {
	case maconfig.AssignmentStrategyKueueAware, maconfig.AssignmentStrategyKueueAwareShadow:
		strategy := kueue.NewKueueAwareStrategy(kueue.KueueAwareParams{
			ClusterCache: in.ClusterCache,
			LocalQueues:  in.LocalQueues,
			State:        in.KueueState,
			Demand:       in.KueueDemand,
			Config:       in.Config,
			Logger:       in.Manager.GetLogger(),
		})
		shadow := in.Config.AssignmentStrategy == maconfig.AssignmentStrategyKueueAwareShadow
		return []kueue.Option{kueue.WithKueueAwareStrategy(strategy, shadow)}
	default:
		return nil
	}
}
