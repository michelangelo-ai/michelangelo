package state

import (
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// PodRequests returns what Kueue charges for a pod, by Kueue's pod-request
// rule: the larger of what the regular and restartable init (sidecar)
// containers request together and what any init container requests with the
// sidecars started before it, plus the pod overhead. A container's missing
// requests default to its limits, as the API server and Kueue default them.
// Ephemeral storage is left out.
func PodRequests(spec *corev1.PodSpec) corev1.ResourceList {
	requests := corev1.ResourceList{}
	for i := range spec.Containers {
		addResources(requests, containerRequests(&spec.Containers[i]))
	}

	sidecars := corev1.ResourceList{}
	initPeak := corev1.ResourceList{}
	for i := range spec.InitContainers {
		c := &spec.InitContainers[i]
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			// a sidecar keeps running alongside the regular containers
			addResources(requests, containerRequests(c))
			addResources(sidecars, containerRequests(c))
			maxResources(initPeak, sidecars)
			continue
		}
		step := containerRequests(c)
		addResources(step, sidecars)
		maxResources(initPeak, step)
	}
	maxResources(requests, initPeak)

	addResources(requests, spec.Overhead)
	delete(requests, corev1.ResourceEphemeralStorage)
	return requests
}

// RayClusterAsk returns what Kueue charges for a KubeRay RayCluster: its head
// pod, and each worker group's pod times the group's replicas and hosts.
func RayClusterAsk(rc *rayv1.RayCluster) corev1.ResourceList {
	ask := PodRequests(&rc.Spec.HeadGroupSpec.Template.Spec)
	for i := range rc.Spec.WorkerGroupSpecs {
		group := &rc.Spec.WorkerGroupSpecs[i]
		// unset replicas default to 0 and unset hosts to 1, as the RayCluster
		// CRD defaults them
		pods := int64(ptr.Deref(group.Replicas, 0)) * int64(max(group.NumOfHosts, 1))
		for name, q := range PodRequests(&group.Template.Spec) {
			q.Mul(pods)
			sum := ask[name]
			sum.Add(q)
			ask[name] = sum
		}
	}
	return ask
}

// containerRequests returns a container's requests, each missing one
// defaulting to the container's limit.
func containerRequests(c *corev1.Container) corev1.ResourceList {
	requests := corev1.ResourceList{}
	for name, limit := range c.Resources.Limits {
		requests[name] = limit.DeepCopy()
	}
	for name, request := range c.Resources.Requests {
		requests[name] = request.DeepCopy()
	}
	return requests
}

// addResources adds add to total.
func addResources(total, add corev1.ResourceList) {
	for name, q := range add {
		sum := total[name]
		sum.Add(q)
		total[name] = sum
	}
}

// maxResources raises each of total's resources to other's, where other's is
// larger.
func maxResources(total, other corev1.ResourceList) {
	for name, q := range other {
		if current, ok := total[name]; !ok || q.Cmp(current) > 0 {
			total[name] = q.DeepCopy()
		}
	}
}
