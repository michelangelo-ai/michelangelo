package k8sengine

// exported constants
const (
	// RayLocalNamespace is the single namespace used for all Ray jobs.
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
