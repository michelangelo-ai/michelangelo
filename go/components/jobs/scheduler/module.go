package scheduler

import (
	"fmt"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	commonscheduler "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/kueue"
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
	Scope              tally.Scope
}

// provide returns the JobQueue selected by jobs.scheduler.backend. The Kueue
// backend wraps the default scheduler rather than replacing it: it validates
// LocalQueue placement for jobs headed to Kueue-managed clusters, then
// delegates, so non-Kueue clusters behave exactly as before.
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
			Delegate:           in.Scheduler,
			Handler:            handler,
			AssignmentStrategy: in.AssignmentStrategy,
			ClusterCache:       in.ClusterCache,
			LocalQueues:        in.LocalQueues,
			Config:             in.Config,
			Scope:              in.Scope,
			Logger:             in.Manager.GetLogger(),
		}), nil
	default:
		return nil, fmt.Errorf(
			"unrecognized jobs.scheduler.backend %q (expected %q or %q)",
			in.Config.Backend, _backendDefault, _backendKueue)
	}
}
