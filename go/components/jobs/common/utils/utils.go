package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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

// GetErrorFromPodStatus attempts to extract an error from the pod status
func GetErrorFromPodStatus(pod *corev1.Pod, containerFilter func(containerStatus corev1.ContainerStatus) bool) *v2pb.PodErrors {
	// First check for container specific errors to get more detailed errors
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerFilter(containerStatus) && containerStatus.State.Terminated != nil && isContainerErrorTheRootCause(containerStatus.State.Terminated) {
			// update the job status with the pod error
			return &v2pb.PodErrors{
				Name:          pod.Name,
				ContainerName: containerStatus.Name,
				ExitCode:      containerStatus.State.Terminated.ExitCode,
				Reason:        containerStatus.State.Terminated.Reason,
				Message:       containerStatus.State.Terminated.Message,
			}
		}
	}

	// next retrieve any error from the pod conditions
	return extractErrorFromPodConditions(pod)
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
