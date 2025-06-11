package framework

import (
	v2beta1pb "michelangelo/api/v2beta1"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
)

type noOpClusterCache struct{}

var _ cluster.RegisteredClustersCache = noOpClusterCache{}

func (c noOpClusterCache) GetClusters(_ cluster.FilterType) []*v2beta1pb.Cluster {
	return nil
}

func (c noOpClusterCache) GetCluster(_ string) *v2beta1pb.Cluster {
	return nil
}
