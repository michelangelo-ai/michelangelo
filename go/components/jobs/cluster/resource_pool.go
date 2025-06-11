package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"code.uber.internal/uberai/michelangelo/shared/gateways/uown"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
)

const (
	_rootAssetName string = "Root"
)

// ResourcePoolCache provides resource pool related information. Resource pools are
// logical abstraction of capacity that is made available to the users of that resource
// pool. The resource pool CRD objects also provides status on the most recent usage
// snapshot of the pool resources.
type ResourcePoolCache interface {
	// GetOwnedResourcePools returns resource pools owned by the uOwn asset
	// with UUID = owningTeamUUID
	GetOwnedResourcePools(owningTeamUUID string) ([]*ResourcePoolInfo, error)

	// GetAuthorizedResourcePools returns resource pools where uOwn asset is in the list of
	// authorized uOwns
	GetAuthorizedResourcePools(authorizedTeamUUID string) ([]*ResourcePoolInfo, error)

	// GetParentOwnedResourcePools returns resource pools owned by the parent
	// uOwn assets of the asset with UUID
	GetParentOwnedResourcePools(owningTeamUUID string) ([]*ResourcePoolInfo, error)

	// GetDefaultResourcePools return the resource pool that can be used by workloads
	// that do not have a resource pool allocated to them
	GetDefaultResourcePools() ([]*ResourcePoolInfo, error)
}

type (
	uOwnUUID         string
	resourcePoolKey  string
	resourcePoolKeys map[resourcePoolKey]struct{}
	uOwnToPoolKeyMap map[uOwnUUID]resourcePoolKeys
)

// resourcePoolCache indexes pools by owning team ID
type resourcePoolCache struct {
	m             sync.RWMutex
	owned         uOwnToPoolKeyMap
	authorized    uOwnToPoolKeyMap
	resourcePools map[resourcePoolKey]*ResourcePoolInfo
	uOwn          uown.Gateway
	log           *zap.Logger
	metrics       *metrics.ControllerMetrics
}

// ResourcePoolCacheParams has parameters to construct ResourcePoolCache
type ResourcePoolCacheParams struct {
	fx.In

	UOwn  uown.Gateway
	Log   *zap.Logger
	Scope tally.Scope
}

// NewResourcePoolCache provides a cached view of the resource pools available & contains "Leaf/Schedulable" Resource pools
// across clusters. This view is refreshed periodically by the cluster controller.
func NewResourcePoolCache(p ResourcePoolCacheParams) ResourcePoolCache {
	return &resourcePoolCache{
		owned:         uOwnToPoolKeyMap{},
		authorized:    uOwnToPoolKeyMap{},
		resourcePools: map[resourcePoolKey]*ResourcePoolInfo{},
		uOwn:          p.UOwn,
		log:           p.Log.With(zap.String(constants.Component, "ResourcePoolCache")),
		metrics:       &metrics.ControllerMetrics{MetricsScope: p.Scope},
	}
}

// ResourcePoolInfo gives information on a resource pool
type ResourcePoolInfo struct {
	ClusterName string
	Pool        infraCrds.ResourcePool
	UpdateTime  metav1.Time
}

// String prints the cluster and the pool info
func (r ResourcePoolInfo) String() string {
	return fmt.Sprintf("[pool:%s cluster:%s]", r.Pool.GetName(), r.ClusterName)
}

func (r ResourcePoolInfo) getResourcePoolKey() resourcePoolKey {
	return resourcePoolKey(r.Pool.Name + "-" + r.ClusterName)
}

const (
	_resourcePool                 = "resource_pool"
	_resourcePoolEnvLabelNotFound = "resource_pool_env_label_not_found"
)

var _ ResourcePoolCache = (*resourcePoolCache)(nil)

func (r *resourcePoolCache) addOrUpdate(
	pool infraCrds.ResourcePool, cluster *v2beta1pb.Cluster) {
	r.m.Lock()
	defer r.m.Unlock()

	poolInfo := &ResourcePoolInfo{
		ClusterName: cluster.Name,
		Pool:        pool,
		UpdateTime:  metav1.Now(),
	}

	poolKey := poolInfo.getResourcePoolKey()

	// The pool's owner or authorized uOwns could have changed. We clear up the existing info.
	if existingInfo, ok := r.resourcePools[poolKey]; ok {
		// It's alright to remove the pool references here because this runs under a RWLock. We add the
		// latest info again to the in-memory map after this section. This approach helps us utilize this
		// central helper util for reference removal and have high coverage for edge cases.
		r.deletePoolReferences(existingInfo)
	}

	// if pool is not valid, remove references in all data structures
	if !r.isValidResourcePool(pool) {
		r.log.Info("existing pool is not valid anymore. deleting it from the cache.")
		r.deletePoolReferences(poolInfo)
		return
	}

	// add/update it to the resource pools cache
	r.resourcePools[poolKey] = poolInfo

	// add/update to the owned pools
	ownedPools := r.owned[uOwnUUID(pool.Spec.OwningTeamID)]
	if ownedPools == nil {
		ownedPools = make(resourcePoolKeys)
		r.owned[uOwnUUID(pool.Spec.OwningTeamID)] = ownedPools
	}
	ownedPools[poolKey] = struct{}{}

	// add/update to the authorized pools
	for _, authorizedUOwnID := range pool.Spec.AuthorizedIdentities {
		authorizedPools := r.authorized[uOwnUUID(authorizedUOwnID)]
		if authorizedPools == nil {
			authorizedPools = make(resourcePoolKeys)
			r.authorized[uOwnUUID(authorizedUOwnID)] = authorizedPools
		}
		authorizedPools[poolKey] = struct{}{}
	}
}

// This helper method should always be called under appropriate locks
func (r *resourcePoolCache) deletePoolReferences(poolInfo *ResourcePoolInfo) {
	poolKey := poolInfo.getResourcePoolKey()

	// delete from pools
	delete(r.resourcePools, poolKey)

	// delete from owned uOwns
	delete(r.owned[uOwnUUID(poolInfo.Pool.Spec.OwningTeamID)], poolKey)

	// delete from authorized uOwns
	for _, authorizedUOwnID := range poolInfo.Pool.Spec.AuthorizedIdentities {
		delete(r.authorized[uOwnUUID(authorizedUOwnID)], poolKey)
	}

}

func (r *resourcePoolCache) delete(
	pool infraCrds.ResourcePool, cluster *v2beta1pb.Cluster) {
	r.m.Lock()
	defer r.m.Unlock()

	r.log.Info("deleting resource pool for given cluster")

	r.deletePoolReferences(&ResourcePoolInfo{
		ClusterName: cluster.Name,
		Pool:        pool,
		UpdateTime:  metav1.Now(),
	})
}

func (r *resourcePoolCache) isValidResourcePool(pool infraCrds.ResourcePool) bool {
	if !utils.IsPresentEnvLabel(pool.Labels) {
		r.log.Info("resource pool's env label is not found",
			zap.String("pool", pool.GetName()))
		r.metrics.MetricsScope.Tagged(map[string]string{
			_resourcePool: pool.GetName()}).
			Counter(_resourcePoolEnvLabelNotFound).Inc(1)
	}

	if pool.Status.Path == "" {
		r.log.Info("resource pool's path is blank",
			zap.String("pool", pool.GetName()))
		return false
	}

	if !pool.Status.IsSchedulable {
		r.log.Info("resource pool is not schedulable",
			zap.String("pool", pool.GetName()))
		return false
	}

	return true
}

func (r *resourcePoolCache) getResourcePools(
	uOwnUUID uOwnUUID, cacheData uOwnToPoolKeyMap) []*ResourcePoolInfo {
	var candidates []*ResourcePoolInfo
	r.m.RLock()
	defer r.m.RUnlock()

	keysData, ok := cacheData[uOwnUUID]
	if !ok || keysData == nil {
		return candidates
	}

	for poolKey := range keysData {
		candidates = append(candidates, r.resourcePools[poolKey])
	}

	return candidates
}

func (r *resourcePoolCache) getPoolsFromOwnedCache(uOwnID string) []*ResourcePoolInfo {
	return r.getResourcePools(uOwnUUID(uOwnID), r.owned)
}

func (r *resourcePoolCache) getPoolsFromAuthorizedCache(uOwnID string) []*ResourcePoolInfo {
	return r.getResourcePools(uOwnUUID(uOwnID), r.authorized)
}

func (r *resourcePoolCache) GetOwnedResourcePools(
	owningTeamUUID string) ([]*ResourcePoolInfo, error) {
	return r.getPoolsFromOwnedCache(owningTeamUUID), nil
}

// GetAuthorizedResourcePools returns resource pools where uOwn asset is in the list of
// authorized uOwns
func (r *resourcePoolCache) GetAuthorizedResourcePools(
	authorizedTeamUUID string) ([]*ResourcePoolInfo, error) {
	return r.getPoolsFromAuthorizedCache(authorizedTeamUUID), nil
}

func (r *resourcePoolCache) GetParentOwnedResourcePools(
	owningTeamUUID string) ([]*ResourcePoolInfo, error) {
	var candidates []*ResourcePoolInfo

	// query the parent uOwn tree
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timer := r.metrics.MetricsScope.Timer(constants.GetUOwnAssetLatency).Start()
	// TODO: Make this a cache to remove querying uOwn from hot path
	resp, err := r.uOwn.GetUOwnAsset(ctx, owningTeamUUID, false)
	if err != nil {
		return nil, err
	}
	timer.Stop()

	asset := resp.Asset.Parent

	// record the resource pools owned by the parents of owningTeamUuid
	for {
		if asset == nil {
			return nil, fmt.Errorf("asset parent was unexpectedly found to be nil")
		}

		if asset.Name == _rootAssetName {
			break
		}

		candidates = append(candidates, r.getPoolsFromOwnedCache(asset.Uuid)...)
		asset = asset.Parent
	}

	return candidates, nil
}

var _defaultPoolUOwnIds = map[string]struct{}{
	"a544c669-dae0-4278-91cd-4c035dec7dd9": {}, // ml-platform-shared-assets
}

func (r *resourcePoolCache) GetDefaultResourcePools() ([]*ResourcePoolInfo, error) {
	var pools []*ResourcePoolInfo
	for uOwn := range _defaultPoolUOwnIds {
		uOwnPools, err := r.GetOwnedResourcePools(uOwn)
		if err != nil {
			return nil, err
		}
		pools = append(pools, uOwnPools...)
	}
	return pools, nil
}

func (r *resourcePoolCache) cleanup(cluster *v2beta1pb.Cluster, pools infraCrds.ResourcePoolList) {
	r.m.Lock()
	defer r.m.Unlock()

	// new resource pools
	freshPoolKeys := make(map[resourcePoolKey]struct{})
	for _, pool := range pools.Items {
		poolInfo := ResourcePoolInfo{
			ClusterName: cluster.Name,
			Pool:        pool,
			UpdateTime:  metav1.Now(),
		}
		freshPoolKeys[poolInfo.getResourcePoolKey()] = struct{}{}
	}

	// delete existing pools that are not present in the fresh set
	for _, pool := range r.resourcePools {
		if _, ok := freshPoolKeys[pool.getResourcePoolKey()]; !ok && pool.ClusterName == cluster.Name {
			r.log.Info("deleting resource pool because it is not present in the latest set of resource pools",
				zap.String("pool", string(pool.getResourcePoolKey())), zap.String("cluster_name", cluster.Name))
			r.deletePoolReferences(pool)
		}
	}
}

func (r *resourcePoolCache) deletePoolsByCluster(cluster *v2beta1pb.Cluster) {
	r.m.Lock()
	defer r.m.Unlock()

	for _, poolInfo := range r.resourcePools {
		if poolInfo.ClusterName == cluster.Name {
			r.deletePoolReferences(poolInfo)
		}
	}
}
