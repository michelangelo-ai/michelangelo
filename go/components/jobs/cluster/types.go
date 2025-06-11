package cluster

import (
	"sync"

	v2beta1pb "michelangelo/api/v2beta1"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
)

// FilterType is the Filter for clusters
type FilterType int

const (
	// ReadyClusters are connected and healthy
	ReadyClusters FilterType = iota
	// UnreadyClusters could be unhealthy and may not be able to run jobs.
	UnreadyClusters
	// AllClusters includes all.
	AllClusters
)

// RegisteredClustersCache is an interface to retrieve the registered clusters.
type RegisteredClustersCache interface {
	// GetClusters returns a list of clusters based on the filter.
	GetClusters(filter FilterType) []*v2beta1pb.Cluster
	// GetCluster returns the cluster with the given name, if found.
	GetCluster(name string) *v2beta1pb.Cluster
}

// Data stores cluster client and previous health check probe results of individual cluster.
type Data struct {
	// clusterStatus is the cluster status as of last sampling.
	// TODO: Do we need this field? We can populate this inside
	// the cachedObj. https://t3.uberinternal.com/browse/MA-20031
	clusterStatus *v2beta1pb.ClusterStatus

	// cachedObj holds the last observer object from apiserver
	cachedObj *v2beta1pb.Cluster
}

// isClusterReady returns true if the cluster is Ready.
func isClusterReady(clusterStatus *v2beta1pb.ClusterStatus) bool {
	if clusterStatus == nil {
		return false
	}

	for _, condition := range clusterStatus.StatusConditions {
		if condition.Type == constants.ClusterReady && condition.Status == v2beta1pb.CONDITION_STATUS_TRUE {
			return true
		}
	}
	return false
}

// concurrent safe map for cluster data
type clusterMap struct {
	m sync.Map
}

func (c *clusterMap) add(cluster string, d *Data) {
	c.m.Store(cluster, d)
}

func (c *clusterMap) get(cluster string) *Data {
	var d *Data
	val, ok := c.m.Load(cluster)
	if ok {
		// guaranteed to be Data
		d = val.(*Data)
	}
	return d
}

func (c *clusterMap) delete(cluster string) {
	c.m.Delete(cluster)
}

func (c *clusterMap) getClustersByFilter(filter FilterType) []*v2beta1pb.Cluster {
	var result []*v2beta1pb.Cluster

	// We always return true from the Range to enable iterating over all clusters present
	// in the sync map.
	switch filter {
	case ReadyClusters:
		c.m.Range(func(_, value any) bool {
			if d, ok := value.(*Data); ok {
				if isClusterReady(d.clusterStatus) {
					result = append(result, d.cachedObj)
				}
			}
			return true
		})
	case UnreadyClusters:
		c.m.Range(func(_, value any) bool {
			if d, ok := value.(*Data); ok {
				if !isClusterReady(d.clusterStatus) {
					result = append(result, d.cachedObj)
				}
			}
			return true
		})
	case AllClusters:
		c.m.Range(func(_, value any) bool {
			if d, ok := value.(*Data); ok {
				result = append(result, d.cachedObj)
			}
			return true
		})
	}

	return result
}
