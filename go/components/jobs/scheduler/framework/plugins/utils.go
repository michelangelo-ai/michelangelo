package plugins

import (
	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	corev1 "k8s.io/api/core/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
)

// getResourceConfigAndUsage returns the appropriate resource configuration slice and usage
// for the given pool and resource SKU, handling both v1 and v2 pool formats
func getResourceConfigAndUsage(poolInfo *cluster.ResourcePoolInfo, resourceSKU string) ([]infraCrds.ResourceConfig, corev1.ResourceList, bool) {
	pool := &poolInfo.Pool

	// Try v2 format first (per-resource-SKU data)
	if pool.Spec.ResourceMap != nil {
		if skuConfig, exists := pool.Spec.ResourceMap[resourceSKU]; exists {
			var usage corev1.ResourceList
			if pool.Status.UsageMap != nil {
				if skuUsage, usageExists := pool.Status.UsageMap[resourceSKU]; usageExists {
					usage = skuUsage
				} else {
					usage = make(corev1.ResourceList)
				}
			} else {
				usage = make(corev1.ResourceList)
			}
			return skuConfig.Resources, usage, true
		}
		// Resource SKU not found in ResourceMap, cannot host this job
		return nil, nil, false
	}

	// Fall back to v1 format (aggregated data)
	if pool.Spec.Resources != nil {
		return pool.Spec.Resources, pool.Status.Usage, true
	}

	// No valid data found
	return nil, nil, false
}

// ConvertResourceMapToResourceList aggregates all resource requirements from a ResourceSKU map into a single ResourceList
func ConvertResourceMapToResourceList(resourcesByKey map[string]corev1.ResourceList) corev1.ResourceList {
	total := make(corev1.ResourceList)

	for _, resourceList := range resourcesByKey {
		total = quotav1.Add(total, resourceList)
	}

	return total
}
