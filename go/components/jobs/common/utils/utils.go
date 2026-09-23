package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsTerminationInfoSet return true if a valid termination spec is set
func IsTerminationInfoSet(
	cluster runtime.Object,
) (bool, error) {
	switch cluster.(type) {
	case *v2pb.RayCluster:
		return cluster.(*v2pb.RayCluster).Spec.GetTermination().GetType() != v2pb.TERMINATION_TYPE_INVALID, nil
	case *v2pb.SparkJob:
		panic("not implemented")
	default:
		return false, fmt.Errorf("invalid job type")
	}
}

// GetProjectNameFromLabels return the value of the project name label
func GetProjectNameFromLabels(labels map[string]string) (string, error) {
	if val, ok := labels[constants.ProjectNameLabelKey]; ok {
		return val, nil
	}
	return "", fmt.Errorf("could not find out the project name from the labels: %v", labels)
}

// ProjectNameForJob returns a job's project identity for control-plane
// decisions: the ma/project-name label when set, otherwise the job's
// namespace — Michelangelo jobs run in their project's namespace, and unlike
// labels the namespace is bound by authorization, so it cannot be forged by
// the job author. Returns "" only when neither is available.
func ProjectNameForJob(labels map[string]string, namespace string) string {
	if name, err := GetProjectNameFromLabels(labels); err == nil && name != "" {
		return name
	}
	return namespace
}

// DefaultLocalQueueTemplate is the LocalQueue naming convention applied when
// jobs.scheduler.kueue.localQueueTemplate is not configured.
const DefaultLocalQueueTemplate = "ma-{project}"

// ResolveLocalQueueName resolves a project's Kueue LocalQueue name on the
// target cluster: an explicit per-project override wins, otherwise "{project}"
// in the template (default "ma-{project}") is substituted with the project
// name. The resolution is control-plane-side by design so the queue a job
// lands in can never be chosen by user-supplied labels.
func ResolveLocalQueueName(cfg maconfig.KueueConfig, project string) string {
	if name, ok := cfg.LocalQueueOverrides[project]; ok && name != "" {
		return name
	}
	tmpl := cfg.LocalQueueTemplate
	if tmpl == "" {
		tmpl = DefaultLocalQueueTemplate
	}
	return strings.ReplaceAll(tmpl, "{project}", project)
}

// GetErrorFromPodStatus attempts to extract an error from the pod status
func GetErrorFromPodStatus(pod *corev1.Pod, containerFilter func(containerStatus corev1.ContainerStatus) bool) *v2pb.PodErrors {
	// First check for container specific errors to get more detailed errors
	if podError := GetContainerErrorFromPodStatus(pod, containerFilter); podError != nil {
		return podError
	}
	// next retrieve any error from the pod conditions
	return extractErrorFromPodConditions(pod)
}

// GetContainerErrorFromPodStatus extracts a container-level failure from a pod
// that is still alive.
//
// Unlike GetErrorFromPodStatus it deliberately does not fall back to the pod
// conditions: a pod that is merely still starting up reports
// ContainersReady=False with reason ContainersNotReady, which is normal
// progress rather than an error. That fallback is only meaningful once the pod
// is gone, which is why it stays in GetErrorFromPodStatus.
func GetContainerErrorFromPodStatus(pod *corev1.Pod, containerFilter func(containerStatus corev1.ContainerStatus) bool) *v2pb.PodErrors {
	// Init containers are scanned alongside the regular ones. KubeRay gives
	// every worker a wait-gcs-ready init container built from the job's own
	// image, and while that one is wedged the regular containers only report
	// PodInitializing — so the init status is the only place the real failure
	// shows up.
	containerStatuses := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	containerStatuses = append(containerStatuses, pod.Status.ContainerStatuses...)
	containerStatuses = append(containerStatuses, pod.Status.InitContainerStatuses...)

	for _, containerStatus := range containerStatuses {
		if containerFilter(containerStatus) && containerStatus.State.Terminated != nil && isContainerErrorTheRootCause(containerStatus.State.Terminated) {
			return &v2pb.PodErrors{
				Name:          pod.Name,
				ContainerName: containerStatus.Name,
				ExitCode:      containerStatus.State.Terminated.ExitCode,
				Reason:        containerStatus.State.Terminated.Reason,
				Message:       containerStatus.State.Terminated.Message,
			}
		}
	}
	// A container that never started has no terminated state. It sits in
	// Waiting indefinitely and its pod is never deleted, so unless the waiting
	// reason is picked up here the failure never reaches the CR at all.
	for _, containerStatus := range containerStatuses {
		if containerFilter(containerStatus) && containerStatus.State.Waiting != nil &&
			waitingStateErrorReasons[containerStatus.State.Waiting.Reason] {
			return &v2pb.PodErrors{
				Name:          pod.Name,
				ContainerName: containerStatus.Name,
				Reason:        containerStatus.State.Waiting.Reason,
				Message:       containerStatus.State.Waiting.Message,
			}
		}
	}
	return nil
}

// waitingStateErrorReasons are container Waiting reasons that mean the
// container failed to start, as opposed to the ordinary startup reasons
// (ContainerCreating, PodInitializing) a healthy pod passes through. Each is
// also listed in terminalPodErrorReasons, which is what lets a cluster whose
// image cannot be pulled converge on FAILED instead of sitting in UNKNOWN.
var waitingStateErrorReasons = map[string]bool{
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CrashLoopBackOff":           true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
}

var _podErrorConditionTypes = map[string]corev1.ConditionStatus{
	"GangTaskFailedPlacement": corev1.ConditionTrue,
	"PlacementTimedOut":       corev1.ConditionTrue,
	"ResourcesPreempted":      corev1.ConditionTrue,
	"ResourcePoolDeleted":     corev1.ConditionTrue,
	"NodeMaintenanceDrain":    corev1.ConditionTrue,

	// Following are standard Kubernetes scheduler conditions.
	// Pod not scheduled is not an error of its own since it's an intermediate state. However, if the pod stays
	// in this state until we hit the timeout, it indicates unavailability of requested resources.
	string(corev1.PodScheduled):    corev1.ConditionFalse,
	string(corev1.ContainersReady): corev1.ConditionFalse,
}

func extractErrorFromPodConditions(pod *corev1.Pod) *v2pb.PodErrors {
	for _, cond := range pod.Status.Conditions {
		if val, ok := _podErrorConditionTypes[string(cond.Type)]; ok && cond.Status == val && cond.Reason != "" {
			return &v2pb.PodErrors{
				Name:    pod.Name,
				Reason:  cond.Reason,
				Message: cond.Message,
			}
		}
	}
	return nil
}

// schedulingErrorReasons are the PodScheduled=False reasons that describe a
// pod the scheduler has given up on, as opposed to one it has not got to yet.
// SchedulingGated is deliberately absent: a gated pod is waiting on Kueue
// admission, which is the queueing working as intended.
var schedulingErrorReasons = map[string]bool{
	corev1.PodReasonUnschedulable:  true,
	corev1.PodReasonSchedulerError: true,
}

// GetSchedulingErrorFromPodStatus surfaces a pod the scheduler could not place.
//
// Such a pod never starts a container, so it has no container statuses at all
// and the container-level extraction finds nothing to report. The actionable
// detail -- "0/1 nodes are available: 1 Insufficient cpu" -- exists only on the
// PodScheduled condition, so without this the CR shows a cluster stuck in
// UNKNOWN with no explanation.
//
// Only PodScheduled is consulted, never the pod conditions at large: a pod that
// is merely starting up reports ContainersReady=False, which is progress rather
// than failure and must not be recorded as an error while the pod is alive.
func GetSchedulingErrorFromPodStatus(pod *corev1.Pod) *v2pb.PodErrors {
	for _, cond := range pod.Status.Conditions {
		if cond.Type != corev1.PodScheduled || cond.Status != corev1.ConditionFalse {
			continue
		}
		if !schedulingErrorReasons[cond.Reason] {
			continue
		}
		return &v2pb.PodErrors{
			Name:    pod.Name,
			Reason:  cond.Reason,
			Message: cond.Message,
		}
	}
	return nil
}

// This method filters out error that are not helpful to determine the root cause of the job
func isContainerErrorTheRootCause(terminatedState *corev1.ContainerStateTerminated) bool {
	isSuccessfulExit := terminatedState.ExitCode == 0

	// SIGTERM with no other reason is typically a side effect of the job shut down and not the root cause of failure
	isSigTerm := terminatedState.ExitCode == 143 &&
		terminatedState.Reason == "Error" && terminatedState.Message == ""

	return !(isSuccessfulExit || isSigTerm)
}

// These are common errors that should be retried.
const (
	_statusUpdateConflictCode = "code = FailedPrecondition"
	_etcdRequestTimedOut      = "etcdserver: request timed out"
	_connectionError          = "code:unavailable message:proxy forward failed"
)

// ErrStatusUpdate is returned when there is an error in updating the status of a job
var ErrStatusUpdate = errors.New(constants.FailureReasonErrorUpdateJobStatus)

// IsRetriableError checks if the error return from the API client can re retried.
// A gRPC error cannot wrap other errors. So we perform a string inspection to determine the actual
// K8s error wrapped by it. See https://github.com/grpc/grpc-go/issues/3115
func IsRetriableError(err error) bool {
	return strings.Contains(err.Error(), _statusUpdateConflictCode) ||
		strings.Contains(err.Error(), _etcdRequestTimedOut) ||
		strings.Contains(err.Error(), _connectionError)
}

// UpdateStatusWithRetries updates the status of the job with conflict handling
// This special handling is required because MA API server wraps the K8s errors in gRPC errors with custom messages
func UpdateStatusWithRetries(ctx context.Context, handler api.Handler, job client.Object,
	applyUpdates func(job client.Object), opts *metav1.UpdateOptions,
) error {
	if err := retry.OnError(retry.DefaultRetry, IsRetriableError, func() error {
		var latestJob client.Object

		// Find out the job type and assign latestJob to an object of that type. This is
		// to make sure that the GET call works fine.
		switch job.(type) {
		case *v2pb.RayJob:
			latestJob = &v2pb.RayJob{}
		case *v2pb.RayCluster:
			latestJob = &v2pb.RayCluster{}
		case *v2pb.SparkJob:
			latestJob = &v2pb.SparkJob{}
		default:
			return fmt.Errorf("invalid job type")
		}

		if err := handler.Get(ctx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, latestJob); err != nil {
			return err
		}
		applyUpdates(latestJob)
		return handler.UpdateStatus(ctx, latestJob, opts)
	}); err != nil {
		// If we exhaust all retries on a re-triable error, then return a special wrapped error to callers to indicate this.
		if IsRetriableError(err) {
			return fmt.Errorf("%w err: %v", ErrStatusUpdate, err)
		}
		return err
	}

	return nil
}

// IsRegionalCluster returns true if the cluster is regional
// Which is defined as a cluster that does not have a zone
func IsRegionalCluster(cluster *v2pb.Cluster) bool {
	return cluster != nil && cluster.Spec.GetZone() == ""
}

var terminalPodErrorReasons = map[string]bool{
	// KubeRay condition-level reasons (from HeadPodReady / ReplicaFailure).
	//
	// "HeadPodNotFound" is intentionally NOT terminal. KubeRay reports
	// HeadPodReady=False / reason=HeadPodNotFound during the brief window
	// between a RayCluster being created and its head pod being scheduled.
	// Treating it as terminal races the ray cluster controller against KubeRay
	// and tears down freshly-created clusters before the head pod ever exists
	// (observed intermittently on the second task of a multi-task Uniflow
	// pipeline). A head pod that genuinely never comes up is still bounded by
	// the workflow's cluster-readiness timeout, and real creation failures
	// surface as FailedCreateHeadPod / FailedCreateWorkerPod below.
	"FailedCreateHeadPod":   true,
	"FailedCreateWorkerPod": true,
	// Kubernetes container-level reasons (if KubeRay exposes them in future versions)
	"ImagePullBackOff":           true,
	"CrashLoopBackOff":           true,
	"OOMKilled":                  true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"ErrImagePull":               true,
	"RunContainerError":          true,
}

// postMortemPodErrorReasons are reasons the kubelet reports about a pod that
// has already been torn down. They diagnose nothing on their own -- the kubelet
// simply no longer has a container to inspect -- so they must never displace an
// error recorded while the pod was still alive.
var postMortemPodErrorReasons = map[string]bool{
	"ContainerStatusUnknown": true,
}

// IsPostMortemPodError reports whether a pod error only describes the aftermath
// of a teardown rather than its cause.
func IsPostMortemPodError(podError *v2pb.PodErrors) bool {
	return postMortemPodErrorReasons[podError.GetReason()]
}

func HasTerminalPodErrors(podErrors []*v2pb.PodErrors) bool {
	for _, pe := range podErrors {
		if terminalPodErrorReasons[pe.Reason] {
			return true
		}
	}
	return false
}
