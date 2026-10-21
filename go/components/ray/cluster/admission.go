package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// admissionObservation maps the state KubeRay reports to what it says about
// admission. KubeRay reports no provisioning state: once Kueue unsuspends a
// cluster, its state is UNKNOWN until it is READY.
func admissionObservation(state v2pb.RayClusterState) jobsutils.AdmissionObservation {
	switch state {
	case v2pb.RAY_CLUSTER_STATE_SUSPENDED:
		return jobsutils.ObservedSuspended
	case v2pb.RAY_CLUSTER_STATE_UNKNOWN, v2pb.RAY_CLUSTER_STATE_PROVISIONING,
		v2pb.RAY_CLUSTER_STATE_READY, v2pb.RAY_CLUSTER_STATE_UNHEALTHY:
		return jobsutils.ObservedUnsuspended
	default:
		return jobsutils.ObservedOther
	}
}

// applyQueued makes the Queued transition that the observed state calls for,
// if any, and returns it (RFC 0a). Queued is True exactly while the cluster
// is suspended, and changes only on a transition, so that its timestamp is
// the time of the transition. kueueTarget is whether the cluster's target is
// Kueue-managed: only there does an admission that no poll saw suspended
// record Queued=False.
func (r *Reconciler) applyQueued(rc *v2pb.RayCluster, observed v2pb.RayClusterState, message string, kueueTarget bool) (*jobsutils.QueuedChange, error) {
	// Iterating the conditions finds an absent Queued condition, where
	// GetCondition would add an Unknown one.
	queued := jobsutils.FindCondition(rc.Status.StatusConditions, QueuedCondition)
	change, ok := jobsutils.NextQueued(queued, admissionObservation(observed), kueueTarget, rc.CreationTimestamp.Time, r.clock())
	if !ok {
		return nil, nil
	}
	metadata, err := change.Metadata()
	if err != nil {
		return nil, err
	}
	if change.Admitted() {
		message = ""
	}
	jobsutils.UpdateCondition(jobsutils.GetCondition(&rc.Status.StatusConditions, QueuedCondition, rc.Generation), jobsutils.ConditionUpdateParams{
		Status:     change.Status,
		Reason:     change.Reason,
		Generation: rc.Generation,
		Message:    message,
		Metadata:   metadata,
	})
	return &change, nil
}

// failIfAdmissionExpired fails a RayCluster whose wait for admission has
// reached the bound, with State FAILED and Succeeded=False/AdmissionTimeout
// (RFC 0a). The termination flow then removes the cluster's compute-side
// object, if any. It reports whether the wait has reached the bound.
func (r *Reconciler) failIfAdmissionExpired(ctx context.Context, rc *v2pb.RayCluster, log logr.Logger) (bool, error) {
	now, bound := r.clock(), r.admissionTimeout()
	if !r.admissionWait(rc).Expired(bound, now) {
		return false, nil
	}
	var waited time.Duration
	written, err := jobsutils.UpdateStatusIfWithRetries(ctx, r, rc, func(obj client.Object) bool {
		c := obj.(*v2pb.RayCluster)
		wait := r.admissionWait(c)
		if r.shouldTerminateCluster(c) || !wait.Expired(bound, now) {
			return false
		}
		waited = now.Sub(wait.Start)
		c.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		jobsutils.UpdateCondition(jobsutils.GetCondition(&c.Status.StatusConditions, SucceededCondition, c.Generation), jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Reason:     constants.AdmissionTimeout,
			Generation: c.Generation,
			Message:    fmt.Sprintf("waited %s for admission, reaching the admission timeout of %s", waited.Round(time.Second), bound),
		})
		return true
	}, &metav1.UpdateOptions{FieldManager: "admissionTimeout"})
	if err != nil {
		return true, fmt.Errorf("record admission timeout: %w", err)
	}
	if written {
		jobsclient.IncAdmissionTimeouts(dispatchMetricsJobType)
		log.Info("ray cluster was not admitted in time, failing it", "waited", waited, "admissionTimeout", bound)
	}
	return true, nil
}

// admissionWait returns the RayCluster's wait for admission.
func (r *Reconciler) admissionWait(rc *v2pb.RayCluster) jobsutils.AdmissionWait {
	return jobsutils.AdmissionWaitOf(r.kueueBackend(), rc.Status.StatusConditions, rc.CreationTimestamp.Time)
}

// admissionTimeout returns the bound on a RayCluster's wait for admission.
func (r *Reconciler) admissionTimeout() time.Duration {
	if timeout := r.schedulerConfig.Kueue.AdmissionTimeout; timeout > 0 {
		return timeout
	}
	return maconfig.DefaultAdmissionTimeout
}

// launchAdmits reports whether launching on target admits the RayCluster:
// under the Kueue backend, dispatch to a cluster that is not Kueue-managed
// counts as admission, since such a cluster never sets Queued.
func (r *Reconciler) launchAdmits(target *v2pb.Cluster) bool {
	return r.kueueBackend() && !jobsutils.IsKueueManaged(target)
}

// observeAdmission records the wait of an admission, which started at start.
// A wait whose start is unknown is not recorded.
func (r *Reconciler) observeAdmission(start time.Time) {
	if start.IsZero() {
		return
	}
	jobsclient.ObserveAdmissionWait(dispatchMetricsJobType, max(r.clock().Sub(start), 0))
}
