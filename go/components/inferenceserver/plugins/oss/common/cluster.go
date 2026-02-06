package common

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// GetTargetClusters returns the target clusters for a given deployment strategy.
// If the deployment strategy is a remote cluster deployment, return the remote cluster targets.
// If the deployment strategy is a control plane cluster deployment, return a single nil cluster target.
func GetTargetClusters(strategy *v2pb.InferenceServerDeploymentStrategy) []*v2pb.ClusterTarget {
	if strategy.GetRemoteClusterDeployment() != nil {
		return strategy.GetRemoteClusterDeployment().GetClusterTargets()
	}
	return []*v2pb.ClusterTarget{nil}
}

func GenerateClusterDisplayName(cluster *v2pb.ClusterTarget) string {
	if cluster == nil {
		return "michelangelo/control-plane-cluster"
	}
	return cluster.ClusterId
}
