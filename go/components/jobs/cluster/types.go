//go:generate mamockgen RegisteredClustersCache
package cluster

import (
	"sync"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
	GetClusters(filter FilterType) []*v2pb.Cluster
	// GetCluster returns the cluster with the given name, if found.
	GetCluster(name string) *v2pb.Cluster
}

// Data stores cluster client and previous health check probe results of individual cluster.
type Data struct {
	// mu protects concurrent access to the fields below
	mu sync.RWMutex

	// clusterStatus is the cluster status as of last sampling.
	clusterStatus *v2pb.ClusterStatus

	// cachedObj holds the last observer object from apiserver
	cachedObj *v2pb.Cluster
}

// SetClusterStatus sets the cluster status in a thread-safe manner.
func (d *Data) SetClusterStatus(status *v2pb.ClusterStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clusterStatus = status
}

// GetClusterStatus gets the cluster status in a thread-safe manner.
func (d *Data) GetClusterStatus() *v2pb.ClusterStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.clusterStatus
}

// UpdateClusterAndStatus updates both the internal cluster status and the cluster object's status in a thread-safe manner.
func (d *Data) UpdateClusterAndStatus(status *v2pb.ClusterStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clusterStatus = status
	if d.cachedObj != nil {
		if status != nil {
			d.cachedObj.Status = *status
		} else {
			d.cachedObj.Status = v2pb.ClusterStatus{}
		}
	}
}

// isClusterReady returns true if the cluster is Ready.
func isClusterReady(clusterStatus *v2pb.ClusterStatus) bool {
	if clusterStatus == nil {
		return false
	}

	for _, condition := range clusterStatus.StatusConditions {
		if condition.Type == constants.ClusterReady && condition.Status == apipb.CONDITION_STATUS_TRUE {
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

func (c *clusterMap) getClustersByFilter(filter FilterType) []*v2pb.Cluster {
	var result []*v2pb.Cluster

	// We always return true from the Range to enable iterating over all clusters present
	// in the sync map.
	switch filter {
	case ReadyClusters:
		c.m.Range(func(_, value any) bool {
			d, ok := value.(*Data)
			if !ok {
				return true
			}
			if isClusterReady(d.GetClusterStatus()) {
				result = append(result, d.cachedObj)
			}
			return true
		})
	case UnreadyClusters:
		c.m.Range(func(_, value any) bool {
			d, ok := value.(*Data)
			if !ok {
				return true
			}
			if !isClusterReady(d.GetClusterStatus()) {
				result = append(result, d.cachedObj)
			}
			return true
		})
	case AllClusters:
		c.m.Range(func(_, value any) bool {
			d, ok := value.(*Data)
			if !ok {
				return true
			}
			result = append(result, d.cachedObj)
			return true
		})
	}

	return result
}
