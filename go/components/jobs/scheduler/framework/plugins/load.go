package plugins

import (
	"context"
	"fmt"
	"sort"

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

	jobResources, err := job.GetResourceRequirement()
	if err != nil {
		return nil, err
	}

	pools := candidates
	sort.Slice(pools, func(i, j int) bool {
		// pool[i] should come before pool[j] if pool[i] can fit the job
		// pool[i] and pool[j] can both fit the job then pool[i] comes before if it has more memory available.
		iCanFit := canResourcePoolFitJob(jobResources, pools[i])
		jCanFit := canResourcePoolFitJob(jobResources, pools[j])

		// if both are same in terms of being able to accommodate the job then compare by dominant resource.
		if (iCanFit && jCanFit) || (!iCanFit && !jCanFit) {
			iAvailable := getAvailable(keyResource, pools[i])
			jAvailable := getAvailable(keyResource, pools[j])

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
		availableCapacity := getAvailable(keyResource, poolInfo)
		log.Info("scored pool", "order", i, "pool", poolInfo.Pool.Name,
			"available_capacity", availableCapacity.String(),
			"can_pool_fit_job", canResourcePoolFitJob(jobResources, poolInfo))
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

// This is a best effort scheme to fit the job in a resource pool. The resource pools usage
// is based on the most recent snapshot taken from the cluster. Therefore, there is a race here
// that we can admit multiple jobs based on this information and when that usage is reflected
// in the snapshot.
func canResourcePoolFitJob(
	requirement corev1.ResourceList,
	poolInfo *cluster.ResourcePoolInfo) bool {
	for _, res := range utils.KnownResources {
		requestedQuantity, ok := requirement[res]
		if !ok {
			continue
		}

		availableQuantity := getAvailable(res, poolInfo)
		if availableQuantity.Cmp(requestedQuantity) < 0 {
			return false
		}
	}

	return true
}

// getAvailable returns the reservation subtracted by usage for a given quantity. This quantity can be
// negative because of elastic resource sharing but that's fine. It will be scored lower by the plugin.
func getAvailable(
	resourceName corev1.ResourceName, poolInfo *cluster.ResourcePoolInfo) resource.Quantity {
	var result resource.Quantity
	for _, resourceConfig := range poolInfo.Pool.Spec.Resources {
		if resourceConfig.Kind == resourceName.String() {
			result = resourceConfig.Reservation
		}
	}

	used := poolInfo.Pool.Status.Usage.Name(resourceName, resource.DecimalSI)

	result.Sub(*used)
	return result
}
