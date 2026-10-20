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

	// cachedObj holds the last observer object from apiserver. It is replaced,
	// never modified in place, so a reader can keep using the pointer it got.
	cachedObj *v2pb.Cluster

	// specVersion counts the in-place spec refreshes, so the result of a probe
	// started before a refresh is not stored for the refreshed spec.
	specVersion uint64
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
	d.setStatusLocked(status)
}

func (d *Data) setStatusLocked(status *v2pb.ClusterStatus) {
	d.clusterStatus = status
	if d.cachedObj != nil {
		obj := d.cachedObj.DeepCopy()
		if status != nil {
			obj.Status = *status
		} else {
			obj.Status = v2pb.ClusterStatus{}
		}
		d.cachedObj = obj
	}
}

// getCachedObj returns the cached cluster object.
func (d *Data) getCachedObj() *v2pb.Cluster {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cachedObj
}

// get returns the cached cluster object together with its status.
func (d *Data) get() (*v2pb.Cluster, *v2pb.ClusterStatus) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cachedObj, d.clusterStatus
}

// snapshot returns the cached cluster object and the spec version it belongs to.
func (d *Data) snapshot() (*v2pb.Cluster, uint64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cachedObj, d.specVersion
}

// refreshSpec swaps obj into the cache in place of the cached object, which it
// returns. The probed status and so the cluster's readiness carry over until
// the refreshed spec is probed.
func (d *Data) refreshSpec(obj *v2pb.Cluster) *v2pb.Cluster {
	d.mu.Lock()
	defer d.mu.Unlock()
	old := d.cachedObj
	refreshed := obj.DeepCopy()
	if old != nil {
		refreshed.Status = old.Status
	}
	d.cachedObj = refreshed
	d.specVersion++
	return old
}

// storeProbeResult stores the status found by a probe of the given spec version
// and returns a copy of the updated object for the status write. It stores
// nothing and returns false if the spec was refreshed during the probe.
func (d *Data) storeProbeResult(specVersion uint64, status *v2pb.ClusterStatus) (*v2pb.Cluster, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if specVersion != d.specVersion {
		return nil, false
	}
	d.setStatusLocked(status)
	return d.cachedObj.DeepCopy(), true
}

// setResourceVersion records the resource version returned by a status write,
// unless the spec was refreshed since, so the next write does not conflict.
func (d *Data) setResourceVersion(specVersion uint64, resourceVersion string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if specVersion != d.specVersion || d.cachedObj == nil || d.cachedObj.ResourceVersion == resourceVersion {
		return
	}
	obj := d.cachedObj.DeepCopy()
	obj.ResourceVersion = resourceVersion
	d.cachedObj = obj
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
			if obj, status := d.get(); isClusterReady(status) {
				result = append(result, obj)
			}
			return true
		})
	case UnreadyClusters:
		c.m.Range(func(_, value any) bool {
			d, ok := value.(*Data)
			if !ok {
				return true
			}
			if obj, status := d.get(); !isClusterReady(status) {
				result = append(result, obj)
			}
			return true
		})
	case AllClusters:
		c.m.Range(func(_, value any) bool {
			d, ok := value.(*Data)
			if !ok {
				return true
			}
			result = append(result, d.getCachedObj())
			return true
		})
	}

	return result
}
