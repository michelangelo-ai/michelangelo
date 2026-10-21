package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// unknownTargetRequeue is the wait for a target whose readiness this
	// process has not established yet (RFC 0f, D40).
	unknownTargetRequeue = 5 * time.Second
	// cleanupRequeue is the wait between terminal-path cleanup tries (D38).
	cleanupRequeue = 30 * time.Second
	// staleCacheBackoffBase and staleCacheBackoffCap bound the wait for the
	// manager's cache to catch up with the live RayCluster (D35).
	staleCacheBackoffBase = time.Second
	staleCacheBackoffCap  = 10 * time.Second

	// backendKueue is the scheduler backend under which a failing dispatch is
	// re-placed (jobs/scheduler's backend names).
	backendKueue = "kueue"
	// dispatchMetricsJobType labels the dispatch and admission metrics.
	dispatchMetricsJobType = "RayCluster"

	clusterCreatedReason        = "ClusterCreated"
	clusterCreationFailedReason = "ClusterCreationFailed"
)

// errCleanupGaveUp is logged when the terminal-path cleanup runs out of time.
var errCleanupGaveUp = errors.New("ray cluster cleanup window expired")

// targetReadiness is a dispatch target's readiness as the gates see it.
type targetReadiness int

const (
	// targetUnknown is a target this process has not probed yet, or one that
	// is not registered while its Cluster object exists: the cache has not
	// caught up.
	targetUnknown targetReadiness = iota
	targetReady
	targetNotReady
	// targetDeregistered is a target with no Cluster object, or one that is
	// being deleted.
	targetDeregistered
)

func (t targetReadiness) String() string {
	switch t {
	case targetUnknown:
		return "unknown"
	case targetReady:
		return "ready"
	case targetNotReady:
		return "not ready"
	default:
		return "deregistered"
	}
}

// dispatchDecision is what follows a failed attempt.
type dispatchDecision int

const (
	decisionRetry dispatchDecision = iota
	decisionReplace
	decisionFail
)

// decideAfterFailedAttempt picks what follows a failed attempt, given the
// state that records it (RFC 0f). Under the Kueue backend, a job is re-placed
// once its target is not Ready or its window is over, unless an attempt may
// have created the object there. Otherwise the job fails at the window's end
// and retries in place before it.
func decideAfterFailedAttempt(kueue bool, state jobsutils.DispatchState, targetReady bool, now time.Time) dispatchDecision {
	expired := state.WindowExpired(now)
	switch {
	case kueue && !state.Ambiguous && (!targetReady || expired):
		return decisionReplace
	case expired:
		return decisionFail
	default:
		return decisionRetry
	}
}

// failedAttempt is a create that failed, or one skipped because its target
// is not Ready or deregistered.
type failedAttempt struct {
	target string
	class  jobsclient.DispatchClass
	cause  error
	// targetReady is whether the target was Ready.
	targetReady bool
	now         time.Time
}

// dispatch creates the RayCluster on its assigned target through RFC 0f's
// gates, and records the outcome. Every status write on this path is guarded:
// it applies only while the job is still dispatched to the target.
func (r *Reconciler) dispatch(ctx context.Context, rc *v2pb.RayCluster, log logr.Logger) (ctrl.Result, error) {
	target := rc.Status.Assignment.GetCluster()
	log = log.WithValues("target", target)
	now := r.clock()
	state, err := jobsutils.DispatchStateFromCondition(jobsutils.FindCondition(rc.Status.StatusConditions, LaunchedCondition))
	if err != nil {
		log.Error(err, "ignoring an undecodable dispatch record")
	}

	// Gate 1: back off between attempts. An attempt's own status write
	// triggers a reconcile, which neither dials nor writes.
	if wait := state.BackoffRemaining(now); wait > 0 {
		log.V(1).Info("backing off before the next create", "requeueAfter", wait)
		return ctrl.Result{RequeueAfter: wait}, nil
	}

	// Gate 2a: wait until the target's readiness is known, counting no attempt.
	readiness, err := r.targetReadiness(ctx, target)
	if err != nil {
		log.Error(err, "could not establish the target's readiness")
		return ctrl.Result{RequeueAfter: unknownTargetRequeue}, nil
	}
	if readiness == targetUnknown {
		log.Info("target readiness unknown, requeue for later")
		return ctrl.Result{RequeueAfter: unknownTargetRequeue}, nil
	}

	// Gate 3: act only on an up-to-date object, so that a launch or
	// re-placement the cache has not caught up with is never redone.
	wait, err := r.staleCacheWait(ctx, rc, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if wait > 0 {
		return ctrl.Result{RequeueAfter: wait}, nil
	}

	// Gate 2b: a target that is not Ready or deregistered is not dialed.
	switch readiness {
	case targetDeregistered:
		if !r.kueueBackend() {
			log.Info("assigned cluster not registered, requeue for later")
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		return r.recordFailedAttempt(ctx, rc, state, failedAttempt{
			target: target, class: jobsclient.DispatchUnreached, now: now,
			cause: fmt.Errorf("target cluster %q is not registered", target),
		}, log)
	case targetNotReady:
		return r.recordFailedAttempt(ctx, rc, state, failedAttempt{
			target: target, class: jobsclient.DispatchUnreached, now: now,
			cause: fmt.Errorf("target cluster %q is not Ready", target),
		}, log)
	}

	targetCluster := r.clusterCache.GetCluster(target)
	if targetCluster == nil {
		// Deregistered since the readiness check.
		log.Info("assigned cluster not registered, requeue for later")
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	log.Info("creating ray cluster via federated client")
	err = r.federatedClient.CreateJobCluster(ctx, rc, targetCluster)
	class := jobsclient.ClassifyDispatchError(err)
	if class == jobsclient.DispatchAlreadyExists {
		class, err = r.resolveAlreadyExists(ctx, rc, targetCluster, log)
	}
	switch class {
	case jobsclient.DispatchSuccess:
		return r.recordLaunched(ctx, rc, target, r.launchAdmits(targetCluster), log)
	case jobsclient.DispatchPermanent:
		log.Error(err, "failed to create ray cluster via federated client")
		return r.failDispatch(ctx, rc, target, err.Error(), nil, state.Ambiguous || r.isLatched(rc.UID), log)
	default:
		log.Error(err, "failed to create ray cluster via federated client", "class", class.String())
		return r.recordFailedAttempt(ctx, rc, state, failedAttempt{
			target: target, class: class, cause: err, targetReady: true, now: now,
		}, log)
	}
}

// resolveAlreadyExists resolves a create that got AlreadyExists from the
// owner of the object found on the target (RFC 0f): the job adopts its own
// object, or one without an owner, and fails at once on another job's.
func (r *Reconciler) resolveAlreadyExists(ctx context.Context, rc *v2pb.RayCluster, targetCluster *v2pb.Cluster, log logr.Logger) (jobsclient.DispatchClass, error) {
	owner, readErr := r.federatedClient.GetJobClusterOwner(ctx, rc, targetCluster)
	class, err := jobsclient.ResolveAlreadyExists(string(rc.UID), owner, readErr)
	if class == jobsclient.DispatchSuccess {
		log.Info("cluster already exists, treating as launched", "owner", owner.Owner, "ownerUID", owner.UID)
	}
	return class, err
}

// recordFailedAttempt adds a failed attempt to the dispatch state and then
// re-places the job, fails it, or retries in place.
func (r *Reconciler) recordFailedAttempt(ctx context.Context, rc *v2pb.RayCluster, state jobsutils.DispatchState, attempt failedAttempt, log logr.Logger) (ctrl.Result, error) {
	state = state.WithAttempt(attempt.now, attempt.class == jobsclient.DispatchAmbiguous || r.isLatched(rc.UID))
	switch decideAfterFailedAttempt(r.kueueBackend(), state, attempt.targetReady, attempt.now) {
	case decisionReplace:
		return r.replace(ctx, rc, state, attempt, log)
	case decisionFail:
		message := fmt.Sprintf("could not create the ray cluster on %s in %d attempts over %s: %v",
			attempt.target, state.Attempts, jobsutils.DispatchWindow, attempt.cause)
		return r.failDispatch(ctx, rc, attempt.target, message, &state, state.Ambiguous, log)
	default:
		return r.retryDispatch(ctx, rc, state, attempt, log)
	}
}

// retryDispatch records the attempt in Launched=False, and requeues the job
// for the end of its backoff.
func (r *Reconciler) retryDispatch(ctx context.Context, rc *v2pb.RayCluster, state jobsutils.DispatchState, attempt failedAttempt, log logr.Logger) (ctrl.Result, error) {
	metadata, err := state.Metadata()
	if err != nil {
		return ctrl.Result{}, err
	}
	written, mismatch, err := r.updateDispatchStatus(ctx, rc, attempt.target, func(c *v2pb.RayCluster) bool {
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		if launched.Status == apipb.CONDITION_STATUS_TRUE {
			return false
		}
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Reason:     clusterCreationFailedReason,
			Generation: c.Generation,
			Message:    attempt.cause.Error(),
			Metadata:   metadata,
		})
		return true
	})
	if err != nil {
		if state.Ambiguous {
			r.latch(rc.UID)
		}
		return ctrl.Result{}, fmt.Errorf("record dispatch attempt: %w", err)
	}
	if !written {
		return r.afterGuardMismatch(ctx, rc, attempt.target, mismatch && state.Ambiguous, log)
	}
	if state.Ambiguous {
		r.unlatch(rc.UID)
	}
	jobsclient.IncCreateRetries(dispatchMetricsJobType, attempt.class)
	wait := state.NextAttemptAt.Sub(attempt.now)
	log.Info("ray cluster creation failed, retrying", "class", attempt.class.String(), "attempts", state.Attempts, "requeueAfter", wait)
	return ctrl.Result{RequeueAfter: wait}, nil
}

// replace hands the job back to the scheduler for a new placement: it clears
// the assignment and sets Scheduled and Launched to Unknown, keeping the
// backoff. The write is skipped if the job launched, or may have been
// created on the target, meanwhile.
func (r *Reconciler) replace(ctx context.Context, rc *v2pb.RayCluster, state jobsutils.DispatchState, attempt failedAttempt, log logr.Logger) (ctrl.Result, error) {
	reason, why := jobsclient.ReplacementReasonWindowExpired, "window expired"
	if !attempt.targetReady {
		reason, why = jobsclient.ReplacementReasonTargetNotReady, "target not ready"
	}
	replaced := state.WithReplacement(attempt.now, attempt.target)
	metadata, err := replaced.Metadata()
	if err != nil {
		return ctrl.Result{}, err
	}
	written, _, err := r.updateDispatchStatus(ctx, rc, attempt.target, func(c *v2pb.RayCluster) bool {
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		persisted, _ := jobsutils.DispatchStateFromCondition(launched)
		if launched.Status == apipb.CONDITION_STATUS_TRUE || persisted.Ambiguous {
			return false
		}
		c.Status.Assignment = nil
		scheduled := jobsutils.GetCondition(&c.Status.StatusConditions, ScheduledCondition, c.Generation)
		jobsutils.UpdateCondition(scheduled, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_UNKNOWN,
			Generation: c.Generation,
			Message:    fmt.Sprintf("re-placing from %s: %s", attempt.target, why),
		})
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_UNKNOWN,
			Generation: c.Generation,
			Message:    attempt.cause.Error(),
			Metadata:   metadata,
		})
		return true
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("re-place ray cluster: %w", err)
	}
	if !written {
		log.Info("not re-placing: the job moved, launched, or may have been created on its target meanwhile")
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	jobsclient.IncReplacements(dispatchMetricsJobType, reason)
	log.Info("re-placing ray cluster", "reason", reason, "replacements", replaced.Replacements)
	return ctrl.Result{}, nil
}

// failDispatch fails the job with Succeeded=False/ClusterCreationFailed. A
// failure at the window's end passes its attempts, which Launched=False
// keeps, so that cleanup knows whether a create may have reached the target;
// a permanent failure leaves Launched as it is. ambiguous is whether a create
// may have reached the target.
func (r *Reconciler) failDispatch(ctx context.Context, rc *v2pb.RayCluster, target, message string, attempts *jobsutils.DispatchState, ambiguous bool, log logr.Logger) (ctrl.Result, error) {
	var params *jobsutils.ConditionUpdateParams
	if attempts != nil {
		metadata, err := attempts.Metadata()
		if err != nil {
			return ctrl.Result{}, err
		}
		params = &jobsutils.ConditionUpdateParams{
			Status:   apipb.CONDITION_STATUS_FALSE,
			Reason:   clusterCreationFailedReason,
			Message:  message,
			Metadata: metadata,
		}
	}
	written, mismatch, err := r.updateDispatchStatus(ctx, rc, target, func(c *v2pb.RayCluster) bool {
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		if launched.Status == apipb.CONDITION_STATUS_TRUE {
			return false
		}
		c.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		succeeded := jobsutils.GetCondition(&c.Status.StatusConditions, SucceededCondition, c.Generation)
		jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: c.Generation,
			Reason:     clusterCreationFailedReason,
			Message:    message,
		})
		if params != nil {
			params.Generation = c.Generation
			jobsutils.UpdateCondition(launched, *params)
		}
		return true
	})
	if err != nil {
		log.Error(err, "failed to update status after creation failure")
		if ambiguous {
			r.latch(rc.UID)
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}
	if !written {
		return r.afterGuardMismatch(ctx, rc, target, mismatch && ambiguous, log)
	}
	if attempts != nil && attempts.Ambiguous {
		r.unlatch(rc.UID)
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// recordLaunched records a successful create: State PROVISIONING and
// Launched=True, which clears the dispatch record. admits is whether the
// launch admits the job (launchAdmits): its wait is recorded once the launch
// is.
func (r *Reconciler) recordLaunched(ctx context.Context, rc *v2pb.RayCluster, target string, admits bool, log logr.Logger) (ctrl.Result, error) {
	written, _, err := r.updateDispatchStatus(ctx, rc, target, func(c *v2pb.RayCluster) bool {
		c.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_TRUE,
			Generation: c.Generation,
			Reason:     clusterCreatedReason,
		})
		return true
	})
	if err != nil {
		log.Error(err, "failed to update launched condition")
		return r.recoverLaunchedWrite(ctx, rc, target, admits, err, log)
	}
	if !written {
		// Only the guard skips this write: the job moved off the target.
		return r.afterGuardMismatch(ctx, rc, target, true, log)
	}
	r.forget(rc.UID)
	if admits {
		r.observeAdmission(rc.CreationTimestamp.Time)
	}
	log.Info("cluster creation initiated")
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// recoverLaunchedWrite handles a Launched=True write that failed. The write
// may still have committed (a 504, an etcd timeout, conflicts outlasting the
// retries), so it deletes nothing unless a live read shows that the job moved
// off the target. Otherwise it records that the object may exist; the next
// create then gets AlreadyExists and adopts it. admits is as for
// recordLaunched.
func (r *Reconciler) recoverLaunchedWrite(ctx context.Context, rc *v2pb.RayCluster, target string, admits bool, writeErr error, log logr.Logger) (ctrl.Result, error) {
	writeErr = fmt.Errorf("record launched ray cluster: %w", writeErr)
	if r.apiReader == nil {
		r.latch(rc.UID)
		return ctrl.Result{}, writeErr
	}
	live := &v2pb.RayCluster{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rc), live); err != nil {
		log.Error(err, "could not read the ray cluster after a failed launched write; keeping a possible creation in memory")
		r.latch(rc.UID)
		return ctrl.Result{}, writeErr
	}
	switch {
	case jobsutils.FindCondition(live.Status.StatusConditions, LaunchedCondition).GetStatus() == apipb.CONDITION_STATUS_TRUE:
		// The write committed.
		r.forget(rc.UID)
		if admits {
			r.observeAdmission(rc.CreationTimestamp.Time)
		}
		return ctrl.Result{}, writeErr
	case !isDispatchedTo(live, target):
		r.removeFromTarget(ctx, rc, target, log)
		return ctrl.Result{}, writeErr
	}

	var alreadyAmbiguous bool
	written, mismatch, err := r.updateDispatchStatus(ctx, rc, target, func(c *v2pb.RayCluster) bool {
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		state, _ := jobsutils.DispatchStateFromCondition(launched)
		alreadyAmbiguous = state.Ambiguous
		if launched.Status == apipb.CONDITION_STATUS_TRUE || state.Ambiguous {
			return false
		}
		state.Ambiguous = true
		metadata, err := state.Metadata()
		if err != nil {
			return false
		}
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     launched.Status,
			Reason:     launched.Reason,
			Generation: c.Generation,
			Message:    launched.Message,
			Metadata:   metadata,
		})
		return true
	})
	switch {
	case err != nil:
		log.Error(err, "could not record a possible creation; keeping it in memory")
		r.latch(rc.UID)
	case written || alreadyAmbiguous:
		r.unlatch(rc.UID)
	case mismatch:
		r.removeFromTarget(ctx, rc, target, log)
	}
	return ctrl.Result{}, writeErr
}

// afterGuardMismatch handles a dispatch write that the guard, or a launch,
// skipped. If a create may have reached the target, it runs the owner-checked
// delete there: the job no longer runs on it.
func (r *Reconciler) afterGuardMismatch(ctx context.Context, rc *v2pb.RayCluster, target string, removeFromTarget bool, log logr.Logger) (ctrl.Result, error) {
	if removeFromTarget {
		log.Info("the job moved off its target after a create that may have reached it")
		r.removeFromTarget(ctx, rc, target, log)
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// removeFromTarget runs the owner-checked delete on a target the job moved
// off, so that a cluster created there does not outlive the job's placement.
func (r *Reconciler) removeFromTarget(ctx context.Context, rc *v2pb.RayCluster, target string, log logr.Logger) {
	targetCluster := r.clusterCache.GetCluster(target)
	if targetCluster == nil {
		log.Info("former target is not registered, nothing to remove", "formerTarget", target)
		return
	}
	if err := r.federatedClient.DeleteJobClusterIfOwned(ctx, rc, targetCluster); err != nil {
		log.Error(err, "could not remove the ray cluster from its former target; it must be removed by hand",
			"formerTarget", target, "namespace", rc.Namespace, "name", rc.Name, "uid", rc.UID)
	}
}

// updateDispatchStatus is a guarded status write: mutate applies to the
// re-read object only while it is still dispatched to target, and returns
// false to write nothing. It reports whether the status was written, and
// whether the guard skipped the last try.
func (r *Reconciler) updateDispatchStatus(ctx context.Context, rc *v2pb.RayCluster, target string, mutate func(*v2pb.RayCluster) bool) (written, mismatch bool, err error) {
	written, err = jobsutils.UpdateStatusIfWithRetries(ctx, r, rc, func(obj client.Object) bool {
		c := obj.(*v2pb.RayCluster)
		mismatch = !isDispatchedTo(c, target)
		if mismatch {
			return false
		}
		return mutate(c)
	}, &metav1.UpdateOptions{})
	return written, mismatch, err
}

// isDispatchedTo reports whether the RayCluster is scheduled to target.
func isDispatchedTo(rc *v2pb.RayCluster, target string) bool {
	return rc.Status.Assignment.GetCluster() == target &&
		jobsutils.FindCondition(rc.Status.StatusConditions, ScheduledCondition).GetStatus() == apipb.CONDITION_STATUS_TRUE
}

// cleanupUnlaunched runs, for a job that never launched, the owner-checked
// delete of what a create may have left on its target: after an ambiguous
// attempt, or one this process could not record. It reports whether the job
// can be finalized. Jobs whose attempts were all unreached or rejected are
// finalized at once, without dialing. The wait for the target to become
// Ready, and the delete's retries, are bounded by the cleanup window, which
// starts at the first cleanup that needs the delete.
func (r *Reconciler) cleanupUnlaunched(ctx context.Context, rc *v2pb.RayCluster, log logr.Logger) (ctrl.Result, bool, error) {
	// The dispatch record decides whether anything may exist, so it is read
	// live rather than from a cache that may not have caught up with it.
	current := rc
	if r.apiReader != nil {
		current = &v2pb.RayCluster{}
		if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rc), current); err != nil {
			return ctrl.Result{}, false, fmt.Errorf("live read of ray cluster: %w", err)
		}
	}
	launched := jobsutils.FindCondition(current.Status.StatusConditions, LaunchedCondition)
	if launched.GetStatus() == apipb.CONDITION_STATUS_TRUE {
		return ctrl.Result{}, true, nil
	}
	state, err := jobsutils.DispatchStateFromCondition(launched)
	if err != nil {
		log.Error(err, "ignoring an undecodable dispatch record")
	}
	target := current.Status.Assignment.GetCluster()
	if target == "" || !(state.Ambiguous || r.isLatched(rc.UID)) {
		return ctrl.Result{}, true, nil
	}
	log = log.WithValues("target", target)

	now := r.clock()
	if state.CleanupExpired(now) {
		log.Error(errCleanupGaveUp, "giving up on a ray cluster that a create may have left on its target; it must be removed by hand",
			"namespace", rc.Namespace, "name", rc.Name, "uid", rc.UID, "cleanupStart", state.CleanupStart)
		return ctrl.Result{}, true, nil
	}
	if state.CleanupStart.IsZero() {
		if err := r.recordCleanupStart(ctx, rc, now); err != nil {
			return ctrl.Result{}, false, fmt.Errorf("record cleanup start: %w", err)
		}
	}

	readiness, err := r.targetReadiness(ctx, target)
	if err != nil {
		log.Error(err, "could not establish the target's readiness")
		return ctrl.Result{RequeueAfter: cleanupRequeue}, false, nil
	}
	switch readiness {
	case targetDeregistered:
		log.Info("target deregistered, nothing to remove")
		return ctrl.Result{}, true, nil
	case targetUnknown, targetNotReady:
		log.Info("waiting for the target before removing what a create may have left", "readiness", readiness.String())
		return ctrl.Result{RequeueAfter: cleanupRequeue}, false, nil
	}
	targetCluster := r.clusterCache.GetCluster(target)
	if targetCluster == nil {
		return ctrl.Result{RequeueAfter: cleanupRequeue}, false, nil
	}
	if err := r.federatedClient.DeleteJobClusterIfOwned(ctx, rc, targetCluster); err != nil {
		log.Error(err, "could not remove what a create may have left on the target, retrying")
		return ctrl.Result{RequeueAfter: cleanupRequeue}, false, nil
	}
	log.Info("removed what a create may have left on the target")
	return ctrl.Result{}, true, nil
}

// recordCleanupStart opens the cleanup window in the Launched condition's
// metadata, once.
func (r *Reconciler) recordCleanupStart(ctx context.Context, rc *v2pb.RayCluster, now time.Time) error {
	_, err := jobsutils.UpdateStatusIfWithRetries(ctx, r, rc, func(obj client.Object) bool {
		c := obj.(*v2pb.RayCluster)
		launched := jobsutils.GetCondition(&c.Status.StatusConditions, LaunchedCondition, c.Generation)
		state, _ := jobsutils.DispatchStateFromCondition(launched)
		if launched.Status == apipb.CONDITION_STATUS_TRUE || !state.CleanupStart.IsZero() {
			return false
		}
		metadata, err := state.WithCleanupStart(now).Metadata()
		if err != nil {
			return false
		}
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     launched.Status,
			Reason:     launched.Reason,
			Generation: c.Generation,
			Message:    launched.Message,
			Metadata:   metadata,
		})
		return true
	}, &metav1.UpdateOptions{})
	return err
}

// targetReadiness evaluates the readiness of the named target.
func (r *Reconciler) targetReadiness(ctx context.Context, name string) (targetReadiness, error) {
	switch r.cachedReadiness(name) {
	case jobscluster.ReadinessReady:
		return targetReady, nil
	case jobscluster.ReadinessNotReady:
		return targetNotReady, nil
	case jobscluster.ReadinessUnknown:
		return targetUnknown, nil
	}
	exists, err := r.clusterObjectExists(ctx, name)
	if err != nil {
		return targetUnknown, err
	}
	if exists {
		return targetUnknown, nil
	}
	return targetDeregistered, nil
}

// cachedReadiness returns the target's readiness from the cluster cache.
// Without a readiness view, a registered target counts as Ready.
func (r *Reconciler) cachedReadiness(name string) jobscluster.Readiness {
	if r.clusterReadiness != nil {
		return r.clusterReadiness.GetClusterReadiness(name)
	}
	if r.clusterCache.GetCluster(name) != nil {
		return jobscluster.ReadinessReady
	}
	return jobscluster.ReadinessNotRegistered
}

// clusterObjectExists reports whether a Cluster of that name exists and is
// not being deleted. Clusters live in their own namespace, not the job's, so
// the lookup goes by name across all namespaces, through the manager client;
// never through the API handler, which falls back to metadata storage.
func (r *Reconciler) clusterObjectExists(ctx context.Context, name string) (bool, error) {
	if r.clusterReader == nil {
		return false, nil
	}
	var clusters v2pb.ClusterList
	if err := r.clusterReader.List(ctx, &clusters); err != nil {
		return false, fmt.Errorf("list clusters: %w", err)
	}
	for i := range clusters.Items {
		if clusters.Items[i].Name == name && clusters.Items[i].DeletionTimestamp.IsZero() {
			return true, nil
		}
	}
	return false, nil
}

// staleCacheWait compares the RayCluster with a live read. While the cache
// is behind, it returns an in-memory wait per RayCluster that doubles from
// 1s to 10s, and resets it once the cache has caught up.
func (r *Reconciler) staleCacheWait(ctx context.Context, rc *v2pb.RayCluster, log logr.Logger) (time.Duration, error) {
	if r.apiReader == nil {
		return 0, nil
	}
	live := &v2pb.RayCluster{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rc), live); err != nil {
		return 0, fmt.Errorf("live read of ray cluster: %w", err)
	}
	if live.ResourceVersion == rc.ResourceVersion {
		r.staleCacheBackoff.Delete(rc.UID)
		return 0, nil
	}
	wait := staleCacheBackoffBase
	if previous, ok := r.staleCacheBackoff.Load(rc.UID); ok {
		wait = min(previous.(time.Duration)*2, staleCacheBackoffCap)
	}
	r.staleCacheBackoff.Store(rc.UID, wait)
	if wait == staleCacheBackoffCap {
		log.Info("cache still behind the live ray cluster, not dispatching",
			"cachedResourceVersion", rc.ResourceVersion, "liveResourceVersion", live.ResourceVersion)
	}
	return wait, nil
}

func (r *Reconciler) kueueBackend() bool {
	return r.schedulerConfig.Backend == backendKueue
}

func (r *Reconciler) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// latch records that a create of the RayCluster may have reached its target
// without that being persisted. Re-placement and the terminal-path cleanup
// treat a latched RayCluster as ambiguous.
func (r *Reconciler) latch(uid types.UID) {
	r.unpersisted.Store(uid, struct{}{})
}

func (r *Reconciler) unlatch(uid types.UID) {
	r.unpersisted.Delete(uid)
}

func (r *Reconciler) isLatched(uid types.UID) bool {
	_, ok := r.unpersisted.Load(uid)
	return ok
}

// forget drops the in-memory dispatch state of a RayCluster that launched or
// was finalized.
func (r *Reconciler) forget(uid types.UID) {
	r.unlatch(uid)
	r.staleCacheBackoff.Delete(uid)
}
