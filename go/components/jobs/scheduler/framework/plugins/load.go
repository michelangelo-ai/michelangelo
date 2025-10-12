package plugins

import (
	"context"
	"fmt"
	"sort"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// LoadScorer scores the resource pools based on the load.
type LoadScorer struct {
	framework.OptionBuilder
}

var _ framework.ScorePlugin = LoadScorer{}

// Name returns the scorer name
func (l LoadScorer) Name() string {
	return "LoadScorer"
}

type scoringPolicy int

const (
	scoreByAvailableMemory scoringPolicy = iota
	scoreByAvailableCPU
	scoreByAvailableDiskSize
	scoreByAvailableGPU
)

// Score scores the candidate clusters. The default policy is to score by available memory
// since that is usually the most demanding resource for most training jobs.
// TODO: Consider adding the default scoring policy to the cluster CRD
func (l LoadScorer) Score(
	_ context.Context,
	job framework.BatchJob,
	candidates []*cluster.ResourcePoolInfo) ([]*cluster.ResourcePoolInfo, error) {
	return l.sortCandidatesByPolicy(job, candidates, scoreByAvailableMemory)
}

func (l LoadScorer) sortCandidatesByPolicy(job framework.BatchJob, candidates []*cluster.ResourcePoolInfo, policy scoringPolicy) ([]*cluster.ResourcePoolInfo, error) {
	keyResource, err := l.getDominantResource(policy)
	if err != nil {
		return nil, err
	}

	pools := candidates
	sort.Slice(pools, func(i, j int) bool {
		// pool[i] should come before pool[j] if pool[i] can fit the job
		// pool[i] and pool[j] can both fit the job then pool[i] comes before if it has more available capacity.
		iCanFit := canResourcePoolFitJob(job, pools[i])
		jCanFit := canResourcePoolFitJob(job, pools[j])

		// if both are same in terms of being able to accommodate the job then compare by dominant resource.
		if (iCanFit && jCanFit) || (!iCanFit && !jCanFit) {
			// Use job-aware scoring that only considers resource keys relevant to the job
			iAvailable := getAvailable(keyResource, pools[i], job)
			jAvailable := getAvailable(keyResource, pools[j], job)

			return iAvailable.Cmp(jAvailable) > 0
		}

		// if result[i] can accommodate the job
		if iCanFit {
			return true
		}

		return false
	},
	)

	log := l.Logger().WithValues("plugin", l.Name()).
		WithValues(constants.Job, job.GetNamespace()+"/"+job.GetName())
	for i, poolInfo := range pools {
		availableCapacity := getAvailable(keyResource, poolInfo, job)
		log.Info("scored pool", "order", i, "pool", poolInfo.Pool.Name,
			"available_capacity", availableCapacity.String(),
			"can_pool_fit_job", canResourcePoolFitJob(job, poolInfo))
	}

	return pools, nil
}

func (l LoadScorer) getDominantResource(policy scoringPolicy) (
	corev1.ResourceName, error) {
	switch policy {
	case scoreByAvailableMemory:
		return corev1.ResourceMemory, nil
	case scoreByAvailableCPU:
		return corev1.ResourceCPU, nil
	case scoreByAvailableDiskSize:
		return corev1.ResourceEphemeralStorage, nil
	case scoreByAvailableGPU:
		return constants.ResourceNvidiaGPU, nil
	}

	return "", fmt.Errorf("unknown policy %v", policy)
}

// canResourcePoolFitJob checks if a resource pool can fit a job.
// For v1 pools (no ResourceMap): uses aggregated logic with job's aggregated resources
// For v2 pools (with ResourceMap): uses component-based approach
func canResourcePoolFitJob(job framework.BatchJob, poolInfo *cluster.ResourcePoolInfo) bool {
	pool := &poolInfo.Pool

	// v1 pools (no ResourceMap): use aggregated logic with job's aggregated resources
	if pool.Spec.ResourceMap == nil {
		jobResourcesByKey, err := job.GetResourceRequirement()
		if err != nil {
			return false
		}
		// Aggregate all resources across resource keys for v1 pool checking
		jobResources := ConvertResourceMapToResourceList(jobResourcesByKey)
		return canV1ResourcePoolFitJob(jobResources, poolInfo)
	}

	// v2 pools (with ResourceMap): use component-based approach for jobs that support it
	return canV2ResourcePoolFitJob(job, poolInfo)
}

// canV1ResourcePoolFitJob checks if a v1 pool (no ResourceMap) can fit a job using aggregated resources
// This is a best effort scheme to fit the job in a resource pool. The resource pools usage
// is based on the most recent snapshot taken from the cluster. Therefore, there is a race here
// that we can admit multiple jobs based on this information and when that usage is reflected
// in the snapshot.

func canV1ResourcePoolFitJob(jobResources corev1.ResourceList, poolInfo *cluster.ResourcePoolInfo) bool {
	for _, res := range utils.KnownResources {
		requestedQuantity, ok := jobResources[res]
		if !ok {
			continue
		}

		availableQuantity := getAvailableV1(res, poolInfo)
		if availableQuantity.Cmp(requestedQuantity) < 0 {
			return false
		}
	}
	return true
}

// canV2ResourcePoolFitJob checks if a v2 resource pool can fit a job
// by checking each component against their specific resource SKUs
func canV2ResourcePoolFitJob(job framework.BatchJob, poolInfo *cluster.ResourcePoolInfo) bool {
	// Get job resource requirements aggregated by resource SKU
	aggregatedRequirements, err := job.GetResourceRequirement()
	if err != nil {
		return false
	}

	// Check each resource SKU's aggregated requirements against pool capacity
	for resourceSKU, requirements := range aggregatedRequirements {
		resourceConfigs, currentUsage, canHost := getResourceConfigAndUsage(poolInfo, resourceSKU)
		if !canHost {
			return false
		}

		// Check if aggregated requirements fit within available capacity
		for _, res := range utils.KnownResources {
			requestedQuantity, ok := requirements[res]
			if !ok {
				continue
			}

			availableQuantity := computeAvailableFromConfig(res, resourceConfigs, currentUsage)
			if availableQuantity.Cmp(requestedQuantity) < 0 {
				return false
			}
		}
	}
	return true
}

// getAvailable gets available capacity for a specific resource considering only
// resource SKUs that are relevant to the given job. This provides accurate constraint-aware scoring:
// - Single resource SKU jobs: returns available capacity for that SKU
// - Heterogeneous jobs: returns minimum available across job-relevant SKUs to reflect bottleneck constraints
func getAvailable(resourceName corev1.ResourceName, poolInfo *cluster.ResourcePoolInfo, job framework.BatchJob) resource.Quantity {
	pool := &poolInfo.Pool

	// v1 pools (no ResourceMap) - return total available capacity
	if pool.Spec.ResourceMap == nil {
		return getAvailableV1(resourceName, poolInfo)
	}

	// v2 pools: Get job's resource requirements to determine relevant resource SKUs
	aggregatedRequirements, err := job.GetResourceRequirement()
	if err != nil || len(aggregatedRequirements) == 0 {
		return resource.Quantity{}
	}

	// Strategy: Constraint-aware scoring
	// 1. For single resource SKU jobs: return available capacity for that SKU
	// 2. For heterogeneous jobs: return minimum available to reflect bottleneck constraints

	if len(aggregatedRequirements) == 1 {
		// Single resource SKU - return its available capacity
		for resourceSKU := range aggregatedRequirements {
			return getAvailableV2(resourceName, poolInfo, resourceSKU)
		}
	}

	// Multiple resource SKUs - use minimum available to reflect constraint bottlenecks
	// This ensures scoring aligns with the reality that ALL resource SKUs must satisfy their requirements
	var minAvailable *resource.Quantity

	for resourceSKU := range aggregatedRequirements {
		available := getAvailableV2(resourceName, poolInfo, resourceSKU)
		if minAvailable == nil || available.Cmp(*minAvailable) < 0 {
			minAvailable = &available
		}
	}

	if minAvailable != nil {
		return *minAvailable
	}

	return resource.Quantity{}
}

// getAvailableV1 returns the reservation subtracted by usage for v1 pools
// This quantity can be negative because of elastic resource sharing but that's fine.
// It will be scored lower by the plugin.
func getAvailableV1(resourceName corev1.ResourceName, poolInfo *cluster.ResourcePoolInfo) resource.Quantity {
	var result resource.Quantity
	for _, resourceConfig := range poolInfo.Pool.Spec.Resources {
		if resourceConfig.Kind == resourceName.String() {
			result = resourceConfig.Reservation
			break
		}
	}

	used := poolInfo.Pool.Status.Usage.Name(resourceName, resource.DecimalSI)
	result.Sub(*used)
	return result
}

// getAvailableV2 returns the reservation subtracted by usage for a given resource SKU
func getAvailableV2(resourceName corev1.ResourceName, poolInfo *cluster.ResourcePoolInfo, resourceSKU string) resource.Quantity {
	resourceConfigs, usage, canHost := getResourceConfigAndUsage(poolInfo, resourceSKU)
	if !canHost {
		return resource.Quantity{}
	}
	return computeAvailableFromConfig(resourceName, resourceConfigs, usage)
}

// computeAvailableFromConfig returns the reservation minus current usage for the given resource
// using a specific resource configuration slice and usage data. This quantity can be negative
// because of elastic resource sharing, which is expected and handled by scoring elsewhere.
func computeAvailableFromConfig(
	resourceName corev1.ResourceName,
	resourceConfigs []infraCrds.ResourceConfig,
	usage corev1.ResourceList) resource.Quantity {
	var result resource.Quantity
	for _, resourceConfig := range resourceConfigs {
		if resourceConfig.Kind == resourceName.String() {
			result = resourceConfig.Reservation
			break
		}
	}

	used := usage.Name(resourceName, resource.DecimalSI)
	result.Sub(*used)
	return result
}
