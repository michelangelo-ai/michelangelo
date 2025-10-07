package k8sengine

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"k8s.io/apimachinery/pkg/runtime"
)

// MapperInterface provides an abstraction for mapping global job objects ( Michelangelo representations )
// to their corresponding local (Kubernetes) representations and extracting
// identifying information such as namespace and name.
type MapperInterface interface {
	// MapGlobalToLocal converts a global job object and its associated cluster object
	// into Kubernetes-native runtime.Objects. It returns the mapped job object,
	// the mapped cluster object, and an error if the mapping fails.
	MapGlobalToLocal(obj runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, runtime.Object, error)

	// GetLocalName extracts the namespace and name from the provided job object.
	// These values are used to identify the corresponding Kubernetes resource.
	GetLocalName(obj runtime.Object) (namespace, name string)
}
