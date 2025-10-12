package plugins

import (
	"context"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PoolLimitFilter filters the resource pool that have resource limits smaller that the job's resource request.
type PoolLimitFilter struct {
	framework.OptionBuilder
}

var _ framework.FilterPlugin = PoolLimitFilter{}

// Name returns the name of the plugin
func (p PoolLimitFilter) Name() string {
	return "PoolLimitFilter"
}

// Filter filters the candidate resource pools and returns the filtered pools.
func (p PoolLimitFilter) Filter(_ context.Context, job framework.BatchJob, candidates []*cluster.ResourcePoolInfo) ([]*cluster.ResourcePoolInfo, error) {
	jobResourcesByKey, err := job.GetResourceRequirement()
	if err != nil {
		return nil, err
	}

	var filteredCandidates []*cluster.ResourcePoolInfo
	for _, candidate := range candidates {
		var jobCanFitWithinPoolLimit bool

		if candidate.Pool.Spec.ResourceMap != nil {
			jobCanFitWithinPoolLimit = p.checkV2PoolLimits(job, candidate)
		} else {
			// Aggregate all resources across resource keys for v1 pool checking
			jobResources := ConvertResourceMapToResourceList(jobResourcesByKey)
			jobCanFitWithinPoolLimit = p.checkV1PoolLimits(job, jobResources, candidate)
		}

		if jobCanFitWithinPoolLimit {
			filteredCandidates = append(filteredCandidates, candidate)
		}
	}

	return filteredCandidates, nil
}

// checkV1PoolLimits checks v1 pools using aggregated resources
func (p PoolLimitFilter) checkV1PoolLimits(job framework.BatchJob, jobResources corev1.ResourceList, candidate *cluster.ResourcePoolInfo) bool {
	log := p.Logger().WithValues("plugin", p.Name()).
		WithValues(constants.Job, job.GetNamespace()+"/"+job.GetName())

	// Check if the resource limit of the candidate pool is larger than the job's resource request
	for _, res := range utils.KnownResources {
		// if the resource is not specified in the job, skip it
		if _, ok := jobResources[res]; !ok {
			continue
		}

		var poolResourceLimit resource.Quantity
		for _, resourceConfig := range candidate.Pool.Spec.Resources {
			if resourceConfig.Kind == res.String() {
				poolResourceLimit = resourceConfig.Limit
				break
			}
		}

		if poolResourceLimit.Cmp(jobResources[res]) < 0 {
			log.Info("Resource pool was filtered out because it's limit is lower than the job requirement",
				"cluster_name", candidate.ClusterName,
				"resource_pool_name", candidate.Pool.Name,
				"resource_name", res.String(),
				"resource_pool_limit", poolResourceLimit.String(),
				"job_requirement", jobResources[res])
			return false
		}
	}
	return true
}

// checkV2PoolLimits checks v2 pools with ResourceMap using a per-resource-SKU approach
func (p PoolLimitFilter) checkV2PoolLimits(job framework.BatchJob, candidate *cluster.ResourcePoolInfo) bool {
	log := p.Logger().WithValues("plugin", p.Name())

	// Get job resource requirements already aggregated by resource SKU
	aggregatedRequirements, err := job.GetResourceRequirement()
	if err != nil {
		log.Info("Failed to get job resource requirements", "error", err)
		return false
	}

	// Validate that pool can host all required resource SKUs
	for resourceSKU, requirements := range aggregatedRequirements {
		if _, exists := candidate.Pool.Spec.ResourceMap[resourceSKU]; !exists {
			log.Info("Resource pool filtered out - unsupported resource SKU",
				"cluster_name", candidate.ClusterName,
				"resource_pool_name", candidate.Pool.Name,
				"required_resource_sku", resourceSKU)
			return false
		}
		if !p.checkResourceSKULimits(requirements, resourceSKU, candidate) {
			return false
		}
	}

	return true
}

// checkResourceSKULimits checks if requirements fit within pool limits for a specific resource SKU
func (p PoolLimitFilter) checkResourceSKULimits(requirements corev1.ResourceList, resourceSKU string, candidate *cluster.ResourcePoolInfo) bool {
	log := p.Logger().WithValues("plugin", p.Name())

	// Get the appropriate resource configuration for this resource SKU
	resourceConfigs, _, canHost := getResourceConfigAndUsage(candidate, resourceSKU)
	if !canHost {
		log.Info("Resource pool filtered out - unsupported resource SKU",
			"cluster_name", candidate.ClusterName,
			"resource_pool_name", candidate.Pool.Name,
			"required_resource_sku", resourceSKU)
		return false
	}

	// Check limits for this resource SKU
	for _, res := range utils.KnownResources {
		requiredQuantity, ok := requirements[res]
		if !ok {
			continue
		}

		var poolResourceLimit resource.Quantity
		for _, resourceConfig := range resourceConfigs {
			if resourceConfig.Kind == res.String() {
				poolResourceLimit = resourceConfig.Limit
				break
			}
		}

		if poolResourceLimit.Cmp(requiredQuantity) < 0 {
			log.Info("Resource pool was filtered out because its limit is lower than the job requirement",
				"cluster_name", candidate.ClusterName,
				"resource_pool_name", candidate.Pool.Name,
				"resource_name", res.String(),
				"resource_pool_limit", poolResourceLimit.String(),
				"job_requirement", requiredQuantity.String(),
				"resource_sku", resourceSKU)
			return false
		}
	}
	return true
}
