package k8sengine

// exported constants
const (
	// RayLocalNamespace is the default compute-cluster namespace for Ray
	// workloads, used when jobs.k8sengine.mapper.rayLocalNamespace is unset.
	RayLocalNamespace = "default"

	// RayAPIVersion is the API version of the RayJob CRD
	RayAPIVersion = "ray.io/v1"

	// RayClusterKind is the kind of the RayCluster CRD
	RayClusterKind = "RayCluster"

	// RayJobKind is the kind of the RayJob CRD
	RayJobKind = "RayJob"

	// RayWorkerNodePrefix is the pod name prefix for Ray Worker nodes
	RayWorkerNodePrefix = "worker-"
)
