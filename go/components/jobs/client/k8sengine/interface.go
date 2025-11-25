package k8sengine

import (
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"k8s.io/apimachinery/pkg/runtime"
)

// MapperInterface provides an abstraction for mapping global job objects ( Michelangelo representations )
// to their corresponding local (Kubernetes) representations and extracting
// identifying information such as namespace and name.
type MapperInterface interface {
	// MapGlobalJobToLocal converts a global job object and its associated cluster object
	// into a Kubernetes-native runtime.Object representing the job.
	// It returns the mapped job object and an error if the mapping fails.
	MapGlobalJobToLocal(jobObject runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error)

	// MapGlobalJobClusterToLocal converts a global cluster object into a Kubernetes-native
	// runtime.Object representing the cluster.
	// It returns the mapped cluster object and an error if the mapping fails.
	MapGlobalJobClusterToLocal(jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error)

	// GetLocalName extracts the namespace and name from the provided job object.
	// These values are used to identify the corresponding Kubernetes resource.
	GetLocalName(obj runtime.Object) (namespace, name string)

	// MapLocalClusterStatusToGlobal converts a local (Kubernetes) cluster status object
	// to the global Michelangelo ClusterStatus representation.
	// It returns the typed ClusterStatus and an error if the conversion fails.
	MapLocalClusterStatusToGlobal(localClusterObject runtime.Object) (*types.JobClusterStatus, error)

	// MapLocalJobStatusToGlobal converts a local (Kubernetes) job status object
	// to the global Michelangelo RayJobStatus representation.
	// It returns the Ray job status and an error if the conversion fails.
	MapLocalJobStatusToGlobal(localJobObject runtime.Object) (*types.JobStatus, error)
}
