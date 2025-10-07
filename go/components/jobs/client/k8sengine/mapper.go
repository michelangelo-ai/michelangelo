package k8sengine

import (
	"fmt"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
)

// Mapper helps to map global to local crds and vice versa
type Mapper struct{}

// MapperResult has Mapper result
type MapperResult struct {
	fx.Out

	Mapper MapperInterface `name:"k8sengineMapper"`
}

const _mapperName = "k8sengineMapper"

// NewMapper constructs the Mapper
func NewMapper() MapperResult {
	return MapperResult{
		Mapper: Mapper{},
	}
}

// MapGlobalToLocal maps the global crd to local crd
func (m Mapper) MapGlobalToLocal(jobObject runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, runtime.Object, error) {
	// localJob and localCluster are optional depending on which objects are provided
	var localJob, localCluster runtime.Object

	// Map job object when provided
	if jobObject != nil {
		switch obj := jobObject.(type) {
		case *v2pb.RayJob:
			lj, err := m.mapRay(obj, jobClusterObject, cluster)
			if err != nil {
				return nil, nil, fmt.Errorf("map ray job: %w", err)
			}
			localJob = lj
		case *v2pb.SparkJob:
			return nil, nil, fmt.Errorf("spark job mapping not implemented: %T", jobObject)
		default:
			return nil, nil, fmt.Errorf("unsupported job object type: %T", jobObject)
		}
	}

	// Map cluster object when provided
	if jobClusterObject != nil {
		switch obj := jobClusterObject.(type) {
		case *v2pb.RayCluster:
			lc, err := m.mapRayCluster(obj)
			if err != nil {
				return nil, nil, fmt.Errorf("map ray cluster: %w", err)
			}
			localCluster = lc
		default:
			return nil, nil, fmt.Errorf("unsupported cluster object type: %T", jobClusterObject)
		}
	}

	return localJob, localCluster, nil
}

// GetLocalName gets the namespaced name of the local crd. This is used by methods that only require the
// namespaced name to perform operations like Delete or Get APIs.
func (m Mapper) GetLocalName(obj runtime.Object) (namespace, name string) {
	switch job := obj.(type) {
	case *v2pb.RayJob:
		namespace = RayLocalNamespace
		name = job.Name
	case *v2pb.SparkJob:
		// Not implemented yet; return empty
		return "", ""
	}
	return
}
