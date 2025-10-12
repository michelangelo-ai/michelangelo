package framework

import (
	computecommonconstants "code.uber.internal/infra/compute/compute-common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	matypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	sharedConstants "code.uber.internal/uberai/michelangelo/shared/constants"
	v1 "k8s.io/api/core/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	v2beta1 "michelangelo/api/v2beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BatchJob is a generic batch job.
type BatchJob interface {
	// SchedulableJob returns basic job information
	matypes.SchedulableJob
	// GetAffinity returns the job affinity
	GetAffinity() *v2beta1.Affinity
	// GetConditions returns the job conditions
	GetConditions() *[]*v2beta1.Condition
	// GetAssignmentInfo returns the assigment info
	GetAssignmentInfo() *v2beta1.AssignmentInfo
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
	GetTerminationSpec() *v2beta1.TerminationSpec
	// IsPreemptibleJob returns true in case of Scheduling Preemptible.
	IsPreemptibleJob() bool
	// GetEnvironmentLabel returns sharedConstants.EnvironmentLabel tag value.
	GetEnvironmentLabel() string
}

// BatchRayJob is the internal type for the global RayJob
type BatchRayJob struct {
	*v2beta1.RayJob
}

var _ BatchJob = BatchRayJob{}

// GetAffinity returns the job affinity
func (r BatchRayJob) GetAffinity() *v2beta1.Affinity {
	return r.Spec.GetAffinity()
}

// GetConditions returns the job conditions
func (r BatchRayJob) GetConditions() *[]*v2beta1.Condition {
	return &r.Status.StatusConditions
}

// GetAssignmentInfo returns the assigment info
func (r BatchRayJob) GetAssignmentInfo() *v2beta1.AssignmentInfo {
	if r.Status.Assignment == nil {
		r.Status.Assignment = &v2beta1.AssignmentInfo{}
	}
	return r.Status.Assignment
}

// GetGeneration returns the job generation
func (r BatchRayJob) GetGeneration() int64 {
	return r.Generation
}

// GetNamespace returns the namespace of the job.
func (r BatchRayJob) GetNamespace() string {
	return r.Namespace
}

// GetName returns the name of the job.
func (r BatchRayJob) GetName() string {
	return r.Name
}

// GetObject returns the underlying client.Object
func (r BatchRayJob) GetObject() client.Object {
	return r.RayJob
}

// GetLabels returns all labels which attached to the job.
func (r BatchRayJob) GetLabels() map[string]string {
	return r.Labels
}

// GetAnnotations returns all the annotations which attached to the job.
func (r BatchRayJob) GetAnnotations() map[string]string {
	return r.Annotations
}

// addResourcesByResourceSKU adds resources for a pod spec with instances to the aggregated map.
// It determines the appropriate resource SKU based on GPU SKU or defaults to CPU-only.
// The function scales the resources by the number of instances and aggregates them into the provided map.
func addResourcesByResourceSKU(aggregated map[string]v1.ResourceList, podSpec *v2beta1.PodSpec, instances int64) error {
	resourceSKU := computecommonconstants.DefaultCPU
	if podSpec.GetResource() != nil {
		if sku := podSpec.GetResource().GetGpuSku(); sku != "" {
			resourceSKU = sku
		} else if podSpec.GetResource().GetGpu() > 0 {
			// For v1 compatibility: GPU jobs without SKU default to RTX5000
			// TODO: Remove once migration to FedV2 is complete
			resourceSKU = string(sharedConstants.RTX5000)
		}
	}

	resourceList, err := utils.ConvertToResourceList(podSpec.GetResource())
	if err != nil {
		return err
	}

	// Scale by instance count
	scaledResourceList, err := utils.ScaleKnownResources(resourceList, instances)
	if err != nil {
		return err
	}
	resourceList = scaledResourceList

	// Initialize aggregated requirements for this resource SKU if not exists
	if _, exists := aggregated[resourceSKU]; !exists {
		aggregated[resourceSKU] = make(v1.ResourceList)
	}

	// Add SKU requirements to aggregated totals
	for resourceName, quantity := range resourceList {
		if existing, exists := aggregated[resourceSKU][resourceName]; exists {
			// Create a copy to avoid modifying the original
			newQuantity := existing.DeepCopy()
			newQuantity.Add(quantity)
			aggregated[resourceSKU][resourceName] = newQuantity
		} else {
			aggregated[resourceSKU][resourceName] = quantity.DeepCopy()
		}
	}

	return nil
}

// GetResourceRequirement returns the resource requirements bucketed by ResourceSKU for the ray job
func (r BatchRayJob) GetResourceRequirement() (map[string]v1.ResourceList, error) {
	aggregated := make(map[string]v1.ResourceList)

	if r.RayJob == nil {
		return aggregated, nil
	}

	// Handle head
	if err := addResourcesByResourceSKU(aggregated, r.Spec.GetHead().GetPod(), 1); err != nil {
		return nil, err
	}

	// Handle workers
	if len(r.Spec.GetWorkers()) > 0 {
		for _, worker := range r.Spec.GetWorkers() {
			if err := addResourcesByResourceSKU(aggregated, worker.GetPod(), int64(worker.GetMinInstances())); err != nil {
				return nil, err
			}
		}
	} else if r.Spec.GetWorker() != nil {
		// Handle single worker
		if err := addResourcesByResourceSKU(aggregated, r.Spec.GetWorker().GetPod(), int64(r.Spec.GetWorker().GetMinInstances())); err != nil {
			return nil, err
		}
	}

	return aggregated, nil
}

// GetUserName returns the username of the user who submitted the job.
func (r BatchRayJob) GetUserName() string {
	return r.Spec.GetUser().GetName()
}

// GetTerminationSpec returns the termination spec
func (r BatchRayJob) GetTerminationSpec() *v2beta1.TerminationSpec {
	return r.Spec.Termination
}

// IsPreemptibleJob returns true in case of Scheduling Preemptible.
func (r BatchRayJob) IsPreemptibleJob() bool {
	return r.Spec.GetScheduling().GetPreemptible()
}

// GetEnvironmentLabel returns sharedConstants.EnvironmentLabel tag value.
func (r BatchRayJob) GetEnvironmentLabel() string {
	return findEnvironmentLabel(r.GetLabels())
}

// GetJobType return the type of the job
func (r BatchRayJob) GetJobType() matypes.JobType {
	return matypes.RayJob
}

// BatchSparkJob is the internal type for the global SparkJob
type BatchSparkJob struct {
	*v2beta1.SparkJob
}

var _ BatchJob = BatchSparkJob{}

// GetAffinity returns the job affinity
func (s BatchSparkJob) GetAffinity() *v2beta1.Affinity {
	return s.Spec.GetAffinity()
}

// GetConditions returns the job conditions
func (s BatchSparkJob) GetConditions() *[]*v2beta1.Condition {
	return &s.Status.StatusConditions
}

// GetAssignmentInfo returns the assigment info
func (s BatchSparkJob) GetAssignmentInfo() *v2beta1.AssignmentInfo {
	if s.Status.Assignment == nil {
		s.Status.Assignment = &v2beta1.AssignmentInfo{}
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
	return map[string]v1.ResourceList{
		computecommonconstants.DefaultCPU: total,
	}, nil
}

// GetUserName returns the username of the user who submitted the job.
func (s BatchSparkJob) GetUserName() string {
	return s.Spec.GetUser().GetName()
}

// GetTerminationSpec returns the termination spec
func (s BatchSparkJob) GetTerminationSpec() *v2beta1.TerminationSpec {
	return s.Spec.Termination
}

// IsPreemptibleJob returns true in case of Scheduling Preemptible.
func (s BatchSparkJob) IsPreemptibleJob() bool {
	return s.Spec.GetScheduling().GetPreemptible()
}

// GetEnvironmentLabel returns sharedConstants.EnvironmentLabel tag value.
func (s BatchSparkJob) GetEnvironmentLabel() string {
	return findEnvironmentLabel(s.GetLabels())
}

// GetJobType return the type of the job
func (s BatchSparkJob) GetJobType() matypes.JobType {
	return matypes.SparkJob
}

func findEnvironmentLabel(labels map[string]string) string {
	if val, ok := labels[sharedConstants.EnvironmentLabel]; ok {
		if val == constants.Production {
			return v2beta1.ENV_TYPE_PRODUCTION.String()
		}
		if val == constants.Development {
			return v2beta1.ENV_TYPE_DEVELOPMENT.String()
		}
		if val == constants.Testing {
			return v2beta1.ENV_TYPE_TESTING.String()
		}
		return labels[sharedConstants.EnvironmentLabel]
	}
	return ""
}
