package framework

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ AssignmentStrategy = ClusterOnlyAssignmentStrategy{}

// ClusterOnlyAssignmentStrategy selects a cluster using affinity.
type ClusterOnlyAssignmentStrategy struct {
	ClusterCache cluster.RegisteredClustersCache
	log          logr.Logger
}

// NewClusterOnlyAssignmentStrategy returns a new ClusterOnlyAssignmentStrategy
func NewClusterOnlyAssignmentStrategy(cache cluster.RegisteredClustersCache, log logr.Logger) AssignmentStrategy {
	return ClusterOnlyAssignmentStrategy{
		ClusterCache: cache,
		log:          log,
	}
}

// Select implements Engine.
//
// Cluster affinity, when the job sets it, is authoritative: the job is either
// assigned to the cluster it named or left unassigned. Falling back to an
// arbitrary cluster would silently run the job somewhere the submitter did not
// ask for, which is worse than not scheduling it at all.
//
// Only a job that names no cluster takes the default, which is the
// lexicographically smallest registered cluster name so that repeated
// scheduling cycles agree on the same choice.
func (e ClusterOnlyAssignmentStrategy) Select(_ context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error) {
	// An explicit cluster name arrives via the resource selector label
	// "michelangelo/cluster-affinity".
	selector := job.GetAffinity().GetResourceAffinity().GetSelector()
	if selector != nil && selector.MatchLabels != nil {
		if name, ok := selector.MatchLabels[constants.ClusterAffinityLabelKey]; ok && name != "" {
			if c := e.ClusterCache.GetCluster(name); c != nil {
				e.log.Info("Assigned to the requested cluster",
					constants.Job, job.GetName(),
					"requested_cluster", name)
				return &v2pb.AssignmentInfo{Cluster: name}, true, constants.AssignmentReasonClusterMatchedByAffinity, nil
			}

			e.log.Info("Requested cluster is not registered, leaving the job unassigned",
				constants.Job, job.GetName(),
				"requested_cluster", name)
			return nil, false, constants.AssignmentReasonAffinityClusterNotFound, nil
		}
	}

	clusters := e.ClusterCache.GetClusters(cluster.AllClusters)
	if len(clusters) == 0 {
		return nil, false, constants.AssignmentReasonNoClustersFound, nil
	}

	// GetClusters ranges over a sync.Map, so the slice order is unspecified and
	// may differ between calls. Take the smallest name rather than whichever
	// element happens to land first.
	defaultCluster := clusters[0].GetName()
	for _, c := range clusters[1:] {
		if name := c.GetName(); name < defaultCluster {
			defaultCluster = name
		}
	}

	e.log.Info("No cluster affinity requested, assigned to the default cluster",
		constants.Job, job.GetName(),
		"default_cluster", defaultCluster)
	return &v2pb.AssignmentInfo{Cluster: defaultCluster}, true, constants.AssignmentReasonClusterDefaultSelected, nil
}
