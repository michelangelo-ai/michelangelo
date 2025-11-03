package framework

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ AssignmentStrategy = ClusterOnlyAssignmentStrategy{}

// ClusterOnlyAssignmentStrategy selects a cluster using affinity.
type ClusterOnlyAssignmentStrategy struct {
	ClusterCache cluster.RegisteredClustersCache
	log          logr.Logger
}

// NewClusterOnlyAssignmentStrategy returns a new ClusterOnlyAssignmentStrategy
func NewClusterOnlyAssignmentStrategy(cache cluster.RegisteredClustersCache) AssignmentStrategy {
	return ClusterOnlyAssignmentStrategy{ClusterCache: cache}
}

// Select implements Engine.
func (e ClusterOnlyAssignmentStrategy) Select(_ context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error) {
	// For OSS MVP: choose the first available cluster, or a specific one
	// if job affinity provides an explicit cluster name via resource selector label
	// "resourcepool.michelangelo/cluster".

	// Prefer explicit cluster by label, else first available.
	selector := job.GetAffinity().GetResourceAffinity().GetSelector()
	if selector != nil && selector.MatchLabels != nil {
		if name, ok := selector.MatchLabels[constants.ClusterAffinityLabelKey]; ok && name != "" {
			if c := e.ClusterCache.GetCluster(name); c != nil {
				return &v2pb.AssignmentInfo{Cluster: name}, true, constants.AssignmentReasonClusterMatchedByAffinity, nil
			}
			e.log.Info("Requested cluster not found, using default selection",
				"job", job.GetName(),
				"requested_cluster", name)
		}
	}

	clusters := e.ClusterCache.GetClusters(cluster.AllClusters)
	if len(clusters) == 0 {
		return nil, false, constants.AssignmentReasonNoClustersFound, nil
	}
	return &v2pb.AssignmentInfo{Cluster: clusters[0].GetName()}, true, constants.AssignmentReasonClusterDefaultSelected, nil
}
