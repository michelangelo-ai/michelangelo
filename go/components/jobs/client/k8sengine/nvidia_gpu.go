package k8sengine

import (
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	corev1 "k8s.io/api/core/v1"
)

// nvidiaGPUTaintKey is the key of the taint NVIDIA's GPU operator puts on GPU
// nodes (nvidia.com/gpu=present:NoSchedule) to keep GPU-free workloads off
// scarce accelerator capacity. NVIDIA reuses the extended-resource name as the
// taint key, so both are derived from one constant here.
const nvidiaGPUTaintKey = string(constants.ResourceNvidiaGPU)

// nvidiaGPUToleration lets a pod schedule onto a node carrying that taint. It
// matches on key and effect rather than on value: "present" is the GPU
// operator's convention, but other driver installers and node pools use other
// values (e.g. "true"), and a pod that asked for a GPU belongs on a GPU node
// under any of them.
var nvidiaGPUToleration = corev1.Toleration{
	Key:      nvidiaGPUTaintKey,
	Operator: corev1.TolerationOpExists,
	Effect:   corev1.TaintEffectNoSchedule,
}

// tolerateNVIDIAGPUTaint adds the NVIDIA GPU toleration to podTemplate when it
// asks for nvidia.com/gpu. A v2 pod template can carry tolerations, but the
// templates Michelangelo generates itself (Uniflow's Ray task, for one) never
// set any, and since the scheduler rather than the caller picks the compute
// cluster, the caller generally cannot know whether that cluster taints its GPU
// nodes. Without this, a GPU cluster sits Pending forever against a tainted GPU
// node pool. Pods that want no GPU are left alone, so the taint keeps doing its
// job of reserving those nodes.
func tolerateNVIDIAGPUTaint(podTemplate *corev1.PodTemplateSpec) {
	if !requestsNVIDIAGPU(&podTemplate.Spec) || toleratesNVIDIAGPUTaint(podTemplate.Spec.Tolerations) {
		return
	}
	// Build a new slice rather than appending in place: podTemplate is a shallow
	// copy of the caller's v2pb pod template, so its slice header still points at
	// the caller's backing array.
	tolerations := make([]corev1.Toleration, 0, len(podTemplate.Spec.Tolerations)+1)
	tolerations = append(tolerations, podTemplate.Spec.Tolerations...)
	podTemplate.Spec.Tolerations = append(tolerations, nvidiaGPUToleration)
}

// requestsNVIDIAGPU reports whether any container in the pod asks for at least
// one nvidia.com/gpu. Limits count alongside requests because Kubernetes
// defaults an extended resource's request from its limit, and init containers
// count because their requests also feed the pod's effective request.
func requestsNVIDIAGPU(podSpec *corev1.PodSpec) bool {
	for _, container := range podSpec.InitContainers {
		if containerRequestsNVIDIAGPU(container) {
			return true
		}
	}
	for _, container := range podSpec.Containers {
		if containerRequestsNVIDIAGPU(container) {
			return true
		}
	}
	return false
}

func containerRequestsNVIDIAGPU(container corev1.Container) bool {
	if qty, ok := container.Resources.Requests[constants.ResourceNvidiaGPU]; ok && !qty.IsZero() {
		return true
	}
	qty, ok := container.Resources.Limits[constants.ResourceNvidiaGPU]
	return ok && !qty.IsZero()
}

// toleratesNVIDIAGPUTaint reports whether the pod already covers the GPU taint,
// either naming it explicitly or tolerating everything through the wildcard
// (empty key + Exists) toleration. A caller who expressed their own intent for
// this taint keeps it; we do not stack a second, redundant entry on top.
func toleratesNVIDIAGPUTaint(tolerations []corev1.Toleration) bool {
	for _, toleration := range tolerations {
		if toleration.Key == nvidiaGPUTaintKey {
			return true
		}
		if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
			return true
		}
	}
	return false
}
