package framework

import (
	"context"
	"slices"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ ExcludingAssignmentStrategy = ClusterOnlyAssignmentStrategy{}

// ClusterOnlyAssignmentStrategy selects a cluster using affinity, else a Ready cluster.
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
func (e ClusterOnlyAssignmentStrategy) Select(_ context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error) {
	// For OSS MVP: choose the first Ready cluster, or a specific one
	// if job affinity provides an explicit cluster name via resource selector label
	// "resourcepool.michelangelo/cluster".

	// Prefer explicit cluster by label (honored whether or not it is Ready),
	// else the first Ready cluster.
	selector := job.GetAffinity().GetResourceAffinity().GetSelector()
	if selector != nil && selector.MatchLabels != nil {
		if name, ok := selector.MatchLabels[constants.ClusterAffinityLabelKey]; ok && name != "" {
			if c := e.ClusterCache.GetCluster(name); c != nil {
				e.log.Info("Assigned to the requested cluster",
					constants.Job, job.GetName(),
					"requested_cluster", name)
				return &v2pb.AssignmentInfo{Cluster: name}, true, constants.AssignmentReasonClusterMatchedByAffinity, nil
			}
			e.log.Info("Requested cluster not found, using default selection",
				constants.Job, job.GetName(),
				"requested_cluster", name)
		}
	}

	clusters := e.ClusterCache.GetClusters(cluster.ReadyClusters)
	if len(clusters) == 0 {
		return nil, false, constants.AssignmentReasonNoClustersFound, nil
	}
	return &v2pb.AssignmentInfo{Cluster: clusters[0].GetName()}, true, constants.AssignmentReasonClusterDefaultSelected, nil
}

// SelectExcluding implements ExcludingAssignmentStrategy: it selects as
// Select does, from a view of the cluster cache whose Ready list leaves out
// the excluded clusters. A pin is looked up by name, so it is kept.
func (e ClusterOnlyAssignmentStrategy) SelectExcluding(ctx context.Context, job BatchJob, excluded []string) (*v2pb.AssignmentInfo, bool, string, error) {
	e.ClusterCache = excludingClusters{RegisteredClustersCache: e.ClusterCache, excluded: excluded}
	return e.Select(ctx, job)
}

// excludingClusters is a view of a cluster cache whose cluster lists leave
// out the excluded clusters. Lookups by name still find them.
type excludingClusters struct {
	cluster.RegisteredClustersCache
	excluded []string
}

// GetClusters returns the cache's clusters for filter, without the excluded
// ones.
func (c excludingClusters) GetClusters(filter cluster.FilterType) []*v2pb.Cluster {
	var kept []*v2pb.Cluster
	for _, cl := range c.RegisteredClustersCache.GetClusters(filter) {
		if !slices.Contains(c.excluded, cl.GetName()) {
			kept = append(kept, cl)
		}
	}
	return kept
}
