package plugins

import (
	"context"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
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
	jobResources, err := job.GetResourceRequirement()
	if err != nil {
		return nil, err
	}

	log := p.Logger().WithValues("plugin", p.Name()).
		WithValues(constants.Job, job.GetNamespace()+"/"+job.GetName())

	var filteredCandidates []*cluster.ResourcePoolInfo
	for _, candidate := range candidates {
		jobCanFitWithinPoolLimit := true
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
				jobCanFitWithinPoolLimit = false
				log.Info("Resource pool was filtered out because it's limit is lower than the job requirement",
					"cluster_name", candidate.ClusterName,
					"resource_pool_name", candidate.Pool.Name,
					"resource_name", res.String(),
					"resource_pool_limit", poolResourceLimit.String(),
					"job_requirement", jobResources[res])
			}
		}

		if jobCanFitWithinPoolLimit {
			filteredCandidates = append(filteredCandidates, candidate)
		}
	}

	return filteredCandidates, nil
}
