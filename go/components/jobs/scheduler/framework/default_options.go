package framework

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
)

type noOpClusterCache struct{}

var _ cluster.RegisteredClustersCache = noOpClusterCache{}

func (c noOpClusterCache) GetClusters(_ cluster.FilterType) []*v2pb.Cluster {
	return nil
}

func (c noOpClusterCache) GetCluster(_ string) *v2pb.Cluster {
	return nil
}
