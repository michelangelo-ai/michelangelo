package k8sengine

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
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

// MapGlobalJobToLocal maps the global job object to local job object
func (m Mapper) MapGlobalJobToLocal(jobObject runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobObject == nil {
		return nil, fmt.Errorf("jobObject cannot be nil")
	}

	switch obj := jobObject.(type) {
	case *v2pb.RayJob:
		localJob, err := m.mapRay(obj, jobClusterObject, cluster)
		if err != nil {
			return nil, fmt.Errorf("map ray job: %w", err)
		}
		return localJob, nil
	case *v2pb.SparkJob:
		return nil, fmt.Errorf("spark job mapping not implemented: %T", jobObject)
	default:
		return nil, fmt.Errorf("unsupported job object type: %T", jobObject)
	}
}

// MapGlobalJobClusterToLocal maps the global cluster object to local cluster object
func (m Mapper) MapGlobalJobClusterToLocal(jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobClusterObject == nil {
		return nil, fmt.Errorf("jobClusterObject cannot be nil")
	}

	switch obj := jobClusterObject.(type) {
	case *v2pb.RayCluster:
		localCluster, err := m.mapRayCluster(obj)
		if err != nil {
			return nil, fmt.Errorf("map ray cluster: %w", err)
		}
		return localCluster, nil
	default:
		return nil, fmt.Errorf("unsupported cluster object type: %T", jobClusterObject)
	}
}

// GetLocalName gets the namespaced name of the local crd. This is used by methods that only require the
// namespaced name to perform operations like Delete or Get APIs.
func (m Mapper) GetLocalName(obj runtime.Object) (namespace, name string) {
	switch job := obj.(type) {
	case *v2pb.RayJob:
		namespace = RayLocalNamespace
		name = job.Name
	case *v2pb.RayCluster:
		namespace = RayLocalNamespace
		name = job.Name
	case *v2pb.SparkJob:
		// Not implemented yet; return empty
		return "", ""
	}
	return
}

// MapLocalClusterStatusToGlobal converts a local (Kubernetes) cluster status object
// to the global Michelangelo ClusterStatus representation.
func (m Mapper) MapLocalClusterStatusToGlobal(localClusterObject runtime.Object) (*types.ClusterStatus, error) {
	if localClusterObject == nil {
		return nil, fmt.Errorf("localClusterObject cannot be nil")
	}

	switch obj := localClusterObject.(type) {
	case *rayv1.RayCluster:
		v2Status := convertRayV1ClusterStatusToV2(obj)
		reason := obj.Status.Reason
		return &types.ClusterStatus{
			Ray:    v2Status,
			Reason: reason,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported cluster object type: %T", localClusterObject)
	}
}
