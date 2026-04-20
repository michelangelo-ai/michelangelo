package framework

import (
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BatchJob is a generic batch job.
type BatchJob interface {
	// SchedulableJob returns basic job information
	matypes.SchedulableJob
	// GetAffinity returns the job affinity
	GetAffinity() *v2pb.Affinity
	// GetConditions returns the job conditions
	GetConditions() *[]*apipb.Condition
	// GetAssignmentInfo returns the assigment info
	GetAssignmentInfo() *v2pb.AssignmentInfo
	// GetObject returns the underlying client.Object
	GetObject() client.Object
	// GetLabels returns all Labels which attached to job.
	GetLabels() map[string]string
	// GetAnnotations returns all Labels which attached to job.
	GetAnnotations() map[string]string
	// GetResourceRequirement returns the resource requirements bucketed by ResourceSKU for the job
	GetResourceRequirement() (map[string]v1.ResourceList, error)
	// GetUserName returns the username of the user who submitted the job.
	GetUserName() string
	// GetTerminationSpec returns the termination spec
	GetTerminationSpec() *v2pb.TerminationSpec
	// IsPreemptibleJob returns true in case of Scheduling Preemptible.
	IsPreemptibleJob() bool
}

// BatchRayCluster is the internal type for the global RayJob
type BatchRayCluster struct {
	*v2pb.RayCluster
}

var _ BatchJob = BatchRayCluster{}

// GetAffinity returns the job affinity
func (r BatchRayCluster) GetAffinity() *v2pb.Affinity {
	// TODO(#611): Add affinity to RayCluster as part of SKU management
	// GetAffinity builds an Affinity from the RayCluster's metadata labels.
	// When SKU management and Scheduling is implemented, we will add affinity to the RayClusterSpec proto.
	if name, ok := r.Labels[constants.ClusterAffinityLabelKey]; ok && name != "" {
		return &v2pb.Affinity{
			ResourceAffinity: &v2pb.ResourceAffinity{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.ClusterAffinityLabelKey: name,
					},
				},
			},
		}
	}
	return nil
}

// GetConditions returns the job conditions
func (r BatchRayCluster) GetConditions() *[]*apipb.Condition {
	return &r.Status.StatusConditions
}

// GetAssignmentInfo returns the assigment info
func (r BatchRayCluster) GetAssignmentInfo() *v2pb.AssignmentInfo {
	if r.Status.Assignment == nil {
		r.Status.Assignment = &v2pb.AssignmentInfo{}
	}
	return r.Status.Assignment
}

// GetGeneration returns the job generation
func (r BatchRayCluster) GetGeneration() int64 {
	return r.Generation
}

// GetNamespace returns the namespace of the job.
func (r BatchRayCluster) GetNamespace() string {
	return r.Namespace
}

// GetName returns the name of the job.
func (r BatchRayCluster) GetName() string {
	return r.Name
}

// GetObject returns the underlying client.Object
func (r BatchRayCluster) GetObject() client.Object {
	return r.RayCluster
}

// GetLabels returns all labels which attached to the cluster.
func (r BatchRayCluster) GetLabels() map[string]string {
	return r.Labels
}

// GetAnnotations returns all the annotations which attached to the job.
func (r BatchRayCluster) GetAnnotations() map[string]string {
	return r.Annotations
}

// getResourceRequestsFromPodSpec sums container resource requests from a core k8s PodSpec.
// If a resource is not requested, it falls back to limits for that resource.
func getResourceRequestsFromPodSpec(podSpec *v1.PodSpec) v1.ResourceList {
	// Start with an empty map; only include resources that are explicitly requested/limited
	summed := v1.ResourceList{
		v1.ResourceCPU:              resource.MustParse("0"),
		v1.ResourceMemory:           resource.MustParse("0"),
		v1.ResourceEphemeralStorage: resource.MustParse("0"),
		constants.ResourceNvidiaGPU: resource.MustParse("0"),
	}

	for _, c := range podSpec.Containers {
		// Prefer requests; fallback to limits per resource
		// CPU
		if qty, ok := c.Resources.Requests[v1.ResourceCPU]; ok {
			addQty(summed, v1.ResourceCPU, qty)
		} else if qty, ok := c.Resources.Limits[v1.ResourceCPU]; ok {
			addQty(summed, v1.ResourceCPU, qty)
		}
		// GPU
		// TODO(#612): Add GPU SKU management
		if qty, ok := c.Resources.Requests[constants.ResourceNvidiaGPU]; ok {
			addQty(summed, constants.ResourceNvidiaGPU, qty)
		} else if qty, ok := c.Resources.Limits[constants.ResourceNvidiaGPU]; ok {
			addQty(summed, constants.ResourceNvidiaGPU, qty)
		}
		// Memory
		if qty, ok := c.Resources.Requests[v1.ResourceMemory]; ok {
			addQty(summed, v1.ResourceMemory, qty)
		} else if qty, ok := c.Resources.Limits[v1.ResourceMemory]; ok {
			addQty(summed, v1.ResourceMemory, qty)
		}
		// Ephemeral storage
		if qty, ok := c.Resources.Requests[v1.ResourceEphemeralStorage]; ok {
			addQty(summed, v1.ResourceEphemeralStorage, qty)
		} else if qty, ok := c.Resources.Limits[v1.ResourceEphemeralStorage]; ok {
			addQty(summed, v1.ResourceEphemeralStorage, qty)
		}
	}
	return summed
}

func addQty(resourceList v1.ResourceList, resourceName v1.ResourceName, quantity resource.Quantity) {
	q := resourceList[resourceName]
	q.Add(quantity)
	resourceList[resourceName] = q
}

// TODO(#612): Placeholder for resource-class key derivation.
// Future: derive from node selectors, tolerations, requests/limits,
// and extended resources (e.g., GPU SKU) to drive resource-aware placement.
func getResourceClassKeyFromPodSpec(_ *v1.PodSpec) string {
	return constants.DefaultCPU
}

func addResourcesByResourceSKU(aggregated map[string]v1.ResourceList, podSpec *v1.PodSpec, instances int64) error {
	if podSpec == nil || instances <= 0 {
		return nil
	}

	bucketKey := getResourceClassKeyFromPodSpec(podSpec)

	// Sum container requests/limits for this PodSpec
	base := getResourceRequestsFromPodSpec(podSpec)

	// Scale using known resource scaler for correctness
	scaled, err := utils.ScaleKnownResources(base, instances)
	if err != nil {
		return err
	}

	if _, exists := aggregated[bucketKey]; !exists {
		aggregated[bucketKey] = make(v1.ResourceList)
	}

	for resourceName, quantity := range scaled {
		if existing, exists := aggregated[bucketKey][resourceName]; exists {
			newQuantity := existing.DeepCopy()
			newQuantity.Add(quantity)
			aggregated[bucketKey][resourceName] = newQuantity
		} else {
			aggregated[bucketKey][resourceName] = quantity.DeepCopy()
		}
	}

	return nil
}

// GetResourceRequirement returns the resource requirements bucketed by ResourceSKU for the ray job
func (r BatchRayCluster) GetResourceRequirement() (map[string]v1.ResourceList, error) {
	aggregated := make(map[string]v1.ResourceList)

	if r.RayCluster == nil {
		return aggregated, nil
	}

	// Handle head (1 instance)
	if err := addResourcesByResourceSKU(aggregated, &r.Spec.Head.Pod.Spec, 1); err != nil {
		return nil, err
	}

	// Handle workers
	if len(r.Spec.GetWorkers()) > 0 {
		for _, worker := range r.Spec.GetWorkers() {
			if err := addResourcesByResourceSKU(aggregated, &worker.GetPod().Spec, int64(worker.GetMinInstances())); err != nil {
				return nil, err
			}
		}
	}

	return aggregated, nil
}

// GetUserName returns the username of the user who submitted the job.
func (r BatchRayCluster) GetUserName() string {
	return r.Spec.GetUser().GetName()
}

// GetTerminationSpec returns the termination spec
func (r BatchRayCluster) GetTerminationSpec() *v2pb.TerminationSpec {
	return r.Spec.Termination
}

// IsPreemptibleJob returns true in case of Scheduling Preemptible.
func (r BatchRayCluster) IsPreemptibleJob() bool {
	return r.Spec.GetScheduling().GetPreemptible()
}

// GetJobType return the type of the job
func (r BatchRayCluster) GetJobType() matypes.JobType {
	return matypes.RayCluster
}

// BatchSparkJob is the internal type for the global SparkJob
type BatchSparkJob struct {
	*v2pb.SparkJob
}

var _ BatchJob = BatchSparkJob{}

// GetAffinity returns the job affinity
func (s BatchSparkJob) GetAffinity() *v2pb.Affinity {
	return s.Spec.GetAffinity()
}

// GetConditions returns the job conditions
func (s BatchSparkJob) GetConditions() *[]*apipb.Condition {
	return &s.Status.StatusConditions
}

// GetAssignmentInfo returns the assigment info
func (s BatchSparkJob) GetAssignmentInfo() *v2pb.AssignmentInfo {
	if s.Status.Assignment == nil {
		s.Status.Assignment = &v2pb.AssignmentInfo{}
	}
	return s.Status.Assignment
}

// GetGeneration returns the job generation
func (s BatchSparkJob) GetGeneration() int64 {
	return s.Generation
}

// GetName returns the name of the job.
func (s BatchSparkJob) GetName() string {
	return s.Name
}

// GetNamespace returns the namespace of the job.
func (s BatchSparkJob) GetNamespace() string {
	return s.Namespace
}

// GetObject returns the underlying client.Object
func (s BatchSparkJob) GetObject() client.Object {
	return s.SparkJob
}

// GetLabels returns all Labels which attached to job.
func (s BatchSparkJob) GetLabels() map[string]string {
	return s.Labels
}

// GetAnnotations returns all Labels which attached to job.
func (s BatchSparkJob) GetAnnotations() map[string]string {
	return s.Annotations
}

// GetResourceRequirement returns the resource requirement for the spark job by adding up the driver and executor(s)
// resources, returned as a map with a single key (DefaultCPU)
func (s BatchSparkJob) GetResourceRequirement() (map[string]v1.ResourceList, error) {
	driverResources, err := utils.ConvertToResourceList(s.Spec.GetDriver().GetPod().GetResource())
	if err != nil {
		return nil, err
	}
	executorResources, err := utils.ConvertToResourceList(s.Spec.GetExecutor().GetPod().GetResource())
	if err != nil {
		return nil, err
	}

	// scale requirements by number of executors
	scaledWorkerRequirements, err := utils.ScaleKnownResources(executorResources, int64(s.Spec.GetExecutor().GetInstances()))
	if err != nil {
		return nil, err
	}

	// determine the combined job requirements
	total := quotav1.Add(driverResources, scaledWorkerRequirements)

	// Return as a map with a single key since Spark jobs don't have heterogeneous resource requirements
	return map[string]v1.ResourceList{constants.DefaultCPU: total}, nil
}

// GetUserName returns the username of the user who submitted the job.
func (s BatchSparkJob) GetUserName() string {
	return s.Spec.GetUser().GetName()
}

// GetTerminationSpec returns the termination spec
func (s BatchSparkJob) GetTerminationSpec() *v2pb.TerminationSpec {
	return s.Spec.Termination
}

// IsPreemptibleJob returns true in case of Scheduling Preemptible.
func (s BatchSparkJob) IsPreemptibleJob() bool {
	return s.Spec.GetScheduling().GetPreemptible()
}

// GetJobType return the type of the job
func (s BatchSparkJob) GetJobType() matypes.JobType {
	return matypes.SparkJob
}
