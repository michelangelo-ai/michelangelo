package k8sengine

import (
	"fmt"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sptr "k8s.io/utils/ptr"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func (m Mapper) mapRay(rayJob *v2pb.RayJob, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
	if jobClusterObject == nil {
		return nil, fmt.Errorf("ray job requires associated RayCluster object")
	}
	rayCluster, ok := jobClusterObject.(*v2pb.RayCluster)
	if !ok {
		return nil, fmt.Errorf("expected *v2pb.RayCluster, got %T", jobClusterObject)
	}
	pod := rayCluster.GetSpec().Head.GetPod()
	kubeRayJob := &rayv1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayJobKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayJob.Name,
			Namespace: RayLocalNamespace,
		},
		Spec: rayv1.RayJobSpec{
			ClusterSelector: map[string]string{
				"ray.io/cluster":      rayCluster.Name,
				"rayClusterNamespace": RayLocalNamespace,
			},
			Entrypoint: rayJob.Spec.Entrypoint,
			// kuberay 1.0 only support SubmitterPodTemplate for configuration submitter pod
			// We need to allow user to configure the submitter pod template via ray task configuration
			// NOTE: add support for v1.2.2 kuberay once we upgrade to newer version
			SubmitterPodTemplate: pod,
		},
	}

	return kubeRayJob, nil
}

func (m Mapper) mapRayCluster(rayCluster *v2pb.RayCluster) (runtime.Object, error) {
	workerGroupSpecs := getWorkerGroupSpecs(rayCluster.GetName(), rayCluster.GetSpec().Workers)
	headGroupSpec := getHeadGroupSpec(rayCluster.GetSpec().Head)
	rayV1Cluster := &rayv1.RayCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       RayClusterKind,
			APIVersion: RayAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rayCluster.Name,
			Namespace: RayLocalNamespace,
		},
		Spec: rayv1.RayClusterSpec{
			HeadGroupSpec:    headGroupSpec,
			RayVersion:       rayCluster.GetSpec().RayVersion,
			WorkerGroupSpecs: workerGroupSpecs,
		},
	}
	return rayV1Cluster, nil
}

func getHeadGroupSpec(head *v2pb.RayHeadSpec) rayv1.HeadGroupSpec {
	return rayv1.HeadGroupSpec{
		ServiceType:    corev1.ServiceType(head.GetServiceType()),
		RayStartParams: head.GetRayStartParams(),
		Template:       k8sptr.Deref(head.GetPod(), corev1.PodTemplateSpec{}),
	}
}

func getWorkerGroupSpecs(clusterName string, workers []*v2pb.RayWorkerSpec) []rayv1.WorkerGroupSpec {
	workerGroupSpecsJSON := make([]rayv1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		wg := rayv1.WorkerGroupSpec{
			GroupName:      RayWorkerNodePrefix + clusterName,
			Replicas:       &workerGroup.MinInstances,
			MinReplicas:    &workerGroup.MinInstances,
			MaxReplicas:    &workerGroup.MaxInstances,
			RayStartParams: workerGroup.RayStartParams,
			Template:       k8sptr.Deref(workerGroup.Pod, corev1.PodTemplateSpec{}),
		}
		workerGroupSpecsJSON[i] = wg
	}
	return workerGroupSpecsJSON
}
