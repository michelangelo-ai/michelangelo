package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/utils/cloud"
	sharedconstants "code.uber.internal/uberai/michelangelo/shared/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
)

// AffinityFilter filters the resource pools based on the job's specified affinity
type AffinityFilter struct {
	framework.OptionBuilder
}

// clusterFilterFunc is an interface which returns if a cluster adheres to a particular property.
type clusterFilterFunc func(cluster *v2beta1pb.Cluster, prop string) bool

var _ framework.FilterPlugin = AffinityFilter{}

// These label keys are used for matching the cluster resource only.
// These are not a part of the resource pool labels.
const (
	ClusterNameLabelKey   = "resourcepool.michelangelo/cluster"
	ClusterRegionLabelKey = "resourcepool.michelangelo/region"
	ClusterZoneLabelKey   = "resourcepool.michelangelo/zone"
)

// ClusterRegionProviderLabelKey is used for matching the cluster resource based on the region provider
const ClusterRegionProviderLabelKey = "resourcepool.michelangelo/region-provider"

// If no cluster affinities are provided, then we use this region to schedule the job.
// This is because the SRC initiative has chosen PHX to be the default region going forward and
// correspondingly some MA tools like MLE exclusively support only the PHX region.
// Among MA customers, very few customer run on the DCA region. Most run on the PHX region.
const _defaultJobRegion = "phx"

// Label keys that specify affinity for cluster matching
var clusterAffinityKeys = map[string]struct{}{
	ClusterNameLabelKey:   {},
	ClusterRegionLabelKey: {},
	ClusterZoneLabelKey:   {},
}

var regionalClusterAffinityKeys = map[string]struct{}{
	ClusterRegionProviderLabelKey: {},
}

const _resourceNameLabelKey = "resourcepool.michelangelo/name"

// shouldUseRegionProvider returns true if any region provider labels are present in the selector
func shouldUseRegionProvider(selector *metav1.LabelSelector) bool {
	if selector == nil || selector.MatchLabels == nil {
		return false
	}

	// Check if region provider is specified in the selector
	regionProvider, hasRegionProvider := selector.MatchLabels[ClusterRegionProviderLabelKey]

	return hasRegionProvider && regionProvider != ""
}

// Name returns the filter name
func (a AffinityFilter) Name() string {
	return "AffinityFilter"
}

// Filter filters the candidate clusters
func (a AffinityFilter) Filter(
	ctx context.Context,
	job framework.BatchJob,
	candidates []*cluster.ResourcePoolInfo) ([]*cluster.ResourcePoolInfo, error) {
	selector, err := a.addCloudZoneToAffinityBasedOnFlipr(ctx, job)
	if err != nil {
		return nil, err
	}

	selector = a.getSelectorWithClusterAffinity(selector)

	// Match resource pool using cluster attributes
	clusterFilteredPools := a.matchClusterSelector(selector, candidates)

	// Match resource pool using other attributes
	var matches []*cluster.ResourcePoolInfo
	for _, c := range clusterFilteredPools {
		if a.matchResourceSelector(selector, &c.Pool) && !a.hasImplicitAntiAffinity(selector, &c.Pool) {
			matches = append(matches, c)
		}
	}
	return matches, nil
}

var (
	_fliprRayJobsInCloud         = "rayJobsInCloud"
	_fliprRunnablePropertyName   = "runnable_name"
	_fliprPipelinePropertyName   = "pipeline_name"
	_fliprProjectCPUPropertyName = "project_name_cpu"
	_fliprProjectGPUPropertyName = "project_name_gpu"
)

var cloudAffinityAnnotations = map[string]string{
	sharedconstants.RunnableNameAnnotation: _fliprRunnablePropertyName,
	sharedconstants.PipelineNameAnnotation: _fliprPipelinePropertyName,
}

func (a AffinityFilter) addCloudZoneToAffinityBasedOnFlipr(ctx context.Context, job framework.BatchJob) (*metav1.LabelSelector, error) {
	// constraints map for flipr check
	constraintsMap := make(map[string]interface{})

	// add project name to constraints maps
	isGpuJob, err := a.isGpuJob(job)
	if err != nil {
		return nil, fmt.Errorf("could not find out if the job is a gpu job, err: %v", err)
	}
	if isGpuJob {
		constraintsMap[_fliprProjectGPUPropertyName] = job.GetNamespace()
	} else {
		constraintsMap[_fliprProjectCPUPropertyName] = job.GetNamespace()
	}

	// add the other constraints based on values found in the job annotations
	for k, v := range cloudAffinityAnnotations {
		if val, ok := job.GetAnnotations()[k]; ok {
			// for pipeline name, we add the project name in front to make it unique
			// since pipeline names are not unique across projects
			if k == sharedconstants.PipelineNameAnnotation {
				val = job.GetNamespace() + "/" + val
			}
			constraintsMap[v] = val
		}
	}

	// get flipr constraints map
	fliprConstraints := a.FliprConstraintsBuilder().GetFliprConstraints(constraintsMap)

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cloudZone, err := a.Flipr().GetStringValue(ctx, _fliprRayJobsInCloud, fliprConstraints, "")
	if err != nil {
		return nil, fmt.Errorf("flipr could not be queried, err: %v", err)
	}

	if cloudZone == "" {
		// return unchanged if flipr cannot be queried or if the result is empty
		return job.GetAffinity().GetResourceAffinity().GetSelector(), nil
	}

	selector := job.GetAffinity().GetResourceAffinity().GetSelector()
	if selector == nil {
		selector = &metav1.LabelSelector{}
	}

	if selector.MatchLabels == nil {
		selector.MatchLabels = make(map[string]string)
	}

	log := a.Logger().WithValues("plugin", a.Name()).
		WithValues(constants.Job, job.GetNamespace()+"/"+job.GetName())
	log.Info("update to cloud zone via flipr", "cloud_zone", cloudZone)

	selector.MatchLabels[ClusterZoneLabelKey] = cloudZone
	return selector, nil
}

func (a AffinityFilter) isGpuJob(job framework.BatchJob) (bool, error) {
	resourceReq, err := job.GetResourceRequirement()
	if err != nil {
		return false, fmt.Errorf("error getting job's resource requirement: %v", err)
	}

	if gpusQ, gpuJob := resourceReq[constants.ResourceNvidiaGPU]; gpuJob && !gpusQ.IsZero() {
		return true, nil
	}
	return false, nil
}

// This method figures out implicit anti affinities to prevent undesirable job to resource pool assignments
func (a AffinityFilter) hasImplicitAntiAffinity(selector *metav1.LabelSelector, pool *infraCrds.ResourcePool) bool {
	return a.hasImplicitGpuAntiAffinity(selector, pool) || a.hasImplicitSpecificGpuAntiAffinity(selector, pool)
}

var _gpuAffinityKey = "resourcepool.michelangelo/support-resource-type-gpu"

// This method checks if a non-GPU job is paired with a GPU pool. Ideally the client should specify anti-affinity
// to get this same behavior. While we have added this anti-affinity to our client in Canvas, some projects with older clients may
// not specify it and get scheduled on GPU pool. This can cause other GPU jobs to get blocked. Example job that got stuck
// https://compute.uberinternal.com/clusters/phx4-kubernetes-batch01/resource-pools/root-uberai-sharedgpupool/jobs/ma-ray-ma-ra-aman-deep-231004-050344-kfl6fg6u
// and corresponding follow-up ticket from Compute https://t3.uberinternal.com/browse/COMPUTE-7313
func (a AffinityFilter) hasImplicitGpuAntiAffinity(selector *metav1.LabelSelector, pool *infraCrds.ResourcePool) bool {
	isCPUOnlyJob := !isGpuLabelValueTrue(selector.MatchLabels)
	isGPUPool := isGpuLabelValueTrue(pool.Labels)
	return isCPUOnlyJob && isGPUPool
}

// This method check that a job does not need special GPU is not scheduled on a resource pool that supports
// special hardware. Instead, it should be run on a pool that uses generic hardware. For example, a job that just requests
// gpu but does not specify a sku should not be run on a special sku like P6000 or A100.
func (a AffinityFilter) hasImplicitSpecificGpuAntiAffinity(selector *metav1.LabelSelector, pool *infraCrds.ResourcePool) bool {
	_, jobNeedSpecificGpu := selector.MatchLabels[constants.ResourcePoolSpecialResourceAlias]
	_, poolSupportSpecificGpu := pool.Labels[constants.ResourcePoolSpecialResourceAlias]
	return !jobNeedSpecificGpu && poolSupportSpecificGpu
}

func isGpuLabelValueTrue(labels map[string]string) bool {
	if lv, ok := labels[_gpuAffinityKey]; ok {
		if boolValue, err := strconv.ParseBool(lv); err == nil && boolValue {
			return true
		}
	}
	return false
}

// Get a selector that has at least one cluster affinity specified.
func (a AffinityFilter) getSelectorWithClusterAffinity(selector *metav1.LabelSelector) *metav1.LabelSelector {
	if a.isClusterAffinityPresent(selector) {
		return selector
	}

	if selector == nil {
		selector = &metav1.LabelSelector{}
	}

	if selector.MatchLabels == nil {
		selector.MatchLabels = make(map[string]string)
	}

	// We add a default cluster region here so that the jobs that are meant to run on the
	// default region (PHX) do not get scheduled on the DCA resource pools. Another way
	// to achieve this is by adding a default region affinity to all the pipelines. We
	// prefer this approach because it's cleaner and avoids large changes to customer projects.
	// TODO: after all projects have migrated to regional clusters (FedV2), set the appropriate default affinities.
	selector.MatchLabels[ClusterRegionLabelKey] = _defaultJobRegion
	return selector
}

func (a AffinityFilter) isClusterAffinityPresent(selector *metav1.LabelSelector) bool {
	if selector == nil || selector.MatchLabels == nil {
		return false
	}

	// Check for standard cluster affinity keys
	for clusterKey := range clusterAffinityKeys {
		if val, ok := selector.MatchLabels[clusterKey]; ok && val != "" {
			return true
		}
	}

	// Check for regional cluster affinity keys
	for clusterKey := range regionalClusterAffinityKeys {
		if regionProvider, ok := selector.MatchLabels[clusterKey]; ok && regionProvider != "" {
			return true
		}
	}

	return false
}

func (a AffinityFilter) matchClusterSelector(selector *metav1.LabelSelector, candidates []*cluster.ResourcePoolInfo) []*cluster.ResourcePoolInfo {
	// Check if we should use region provider-based routing based on selector labels
	useRegionProvider := shouldUseRegionProvider(selector)

	// Apply filters in order based on whether we're using region provider-based routing
	if useRegionProvider {
		return a.matchClusterSelectorOnRegionProvider(selector, candidates)
	}

	// Traditional region/zone matching when not using region provider-based routing
	return a.matchClusterSelectorOnZone(selector, candidates)
}

func (a AffinityFilter) matchClusterSelectorOnZone(selector *metav1.LabelSelector, candidates []*cluster.ResourcePoolInfo) []*cluster.ResourcePoolInfo {
	// Match using region key of cluster
	matches := a.filterByClusterProperty(selector, ClusterRegionLabelKey, candidates, func(cluster *v2beta1pb.Cluster, region string) bool {
		return strings.EqualFold(cluster.Spec.GetRegion(), region)
	})

	// Match using zone key of cluster
	matches = a.filterByClusterProperty(selector, ClusterZoneLabelKey, matches, func(cluster *v2beta1pb.Cluster, zone string) bool {
		return strings.EqualFold(cluster.Spec.GetZone(), zone)
	})

	// Filter out cloud zones if required
	matches = a.dropCloudZones(selector, matches)

	// Match using cluster name
	return a.filterByClusterProperty(selector, ClusterNameLabelKey, matches, func(cluster *v2beta1pb.Cluster, name string) bool {
		return strings.EqualFold(cluster.GetName(), name)
	})
}

func (a AffinityFilter) matchClusterSelectorOnRegionProvider(selector *metav1.LabelSelector, candidates []*cluster.ResourcePoolInfo) []*cluster.ResourcePoolInfo {
	regionProvider := selector.MatchLabels[ClusterRegionProviderLabelKey]
	typedRegionProvider := cloud.RegionProvider(regionProvider)
	region := cloud.GetRegion(typedRegionProvider)
	provider := cloud.GetProvider(typedRegionProvider)

	dcType, ok := cloud.GetDCTypeFromProvider(provider)
	if !ok {
		return []*cluster.ResourcePoolInfo{}
	}

	matches := []*cluster.ResourcePoolInfo{}
	for _, pool := range candidates {
		cluster := a.ClusterCache().GetCluster(pool.ClusterName)
		// Match region and provider affinities if the cluster is regional
		if utils.IsRegionalCluster(cluster) &&
			strings.EqualFold(cluster.Spec.GetRegion(), region) &&
			cluster.Spec.GetDc() == dcType {
			matches = append(matches, pool)
		}
	}

	return matches
}

// Cloud zones can only be used if the zone affinity is explicitly specified.
// This is to make sure that these zones can still be used in production while not getting allocated
// to general traffic. This helps rollout new capabilities in a controlled manner.
func (a AffinityFilter) dropCloudZones(selector *metav1.LabelSelector,
	candidates []*cluster.ResourcePoolInfo) []*cluster.ResourcePoolInfo {

	// if the zone is specified, don't change the candidate set
	if _, ok := selector.MatchLabels[ClusterZoneLabelKey]; ok {
		return candidates
	}

	var filtered []*cluster.ResourcePoolInfo
	for _, pool := range candidates {
		cluster := a.ClusterCache().GetCluster(pool.ClusterName)

		// Cluster could be nil if it deleted
		if cluster == nil {
			continue
		}

		// Skip if it's a cloud zone using cloud helper function
		if cloud.IsCloudProvider(cloud.Zone(cluster.Spec.GetZone())) {
			continue
		}

		filtered = append(filtered, pool)
	}

	return filtered
}

// match the affinity with the properties of the cluster in which the resource pool lives
func (a AffinityFilter) filterByClusterProperty(selector *metav1.LabelSelector, prop string,
	candidates []*cluster.ResourcePoolInfo, filterFunc clusterFilterFunc) []*cluster.ResourcePoolInfo {
	value, ok := selector.MatchLabels[prop]
	if !ok || value == "" {
		return candidates
	}

	var filtered []*cluster.ResourcePoolInfo
	for _, pool := range candidates {
		cluster := a.ClusterCache().GetCluster(pool.ClusterName)
		// Cluster could be nil if it deleted
		if cluster != nil && filterFunc(cluster, value) {
			filtered = append(filtered, pool)
		}
	}

	return filtered
}

// Used to match job with a resource pool using a selector. See
// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
// for more details.
func (a AffinityFilter) matchResourceSelector(selector *metav1.LabelSelector, pool *infraCrds.ResourcePool) bool {
	for k, v := range selector.MatchLabels {
		if _, ok := clusterAffinityKeys[k]; ok {
			continue
		}

		// check spec
		if k == _resourceNameLabelKey {
			if !strings.EqualFold(v, pool.Status.Path) {
				return false
			}
			continue
		}

		// check labels
		lv, ok := pool.GetLabels()[k]
		if !ok || !strings.EqualFold(lv, v) {
			return false
		}
	}

	// TODO: Add support for clusterSelector.MatchExpressions
	return true
}
