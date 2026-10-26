package job

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/watch"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const _eventHandlerTimeout = 10 * time.Second

// getFederatedWatcher creates a federated watcher for the SparkJob controller.
//
// It watches a single resource on every ready compute cluster: the
// SparkApplication this control plane projected there, to propagate the
// application's status onto the global v2pb.SparkJob. The watch is how that
// status normally arrives; Reconcile reads it from the member cluster itself
// only once a launched job's status has gone sparkStatusProbeInterval without
// a change, and not while the job's cluster is held (holdCluster), as a
// backstop for events the watch never delivered.
//
// The driver and executor pods carry the control-plane labels too, so a Pod
// watch would be possible, but there is none: nothing on v2pb.SparkJobStatus
// is derived from individual pods. That leaves a gap. The Spark Operator
// reports a driver pod that never starts, one stuck in ErrImagePull,
// ImagePullBackOff or CreateContainerConfigError, as SUBMITTED, with no error
// and no time limit, so the watch records SUBMITTED, every probe reads it
// again and writes nothing, and the SparkJob never gets an outcome. Ray's pod
// watch fails a RayCluster for these reasons (utils.terminalPodErrorReasons);
// Spark has no equivalent yet.
func (r *Reconciler) getFederatedWatcher() watch.FederatedWatcher {
	return watch.NewFederatedWatcher(watch.FederatedWatcherParams{
		ClusterCache:    r.clusterCache,
		FederatedClient: r.federatedClient,
		Logger:          r.logger.WithValues("resource", "sparkapplication"),
		WatcherParams:   []*jobsclient.WatcherParams{r.sparkApplicationWatcherParams()},
		Scope:           r.metricsScope,
	})
}

// sparkApplicationWatcherParams describes the SparkApplication watch. It is its
// own method so a test can assert on the selector without standing up a
// federated watcher.
func (r *Reconciler) sparkApplicationWatcherParams() *jobsclient.WatcherParams {
	return &jobsclient.WatcherParams{
		ResourceName: constants.KubeSparkResource,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				// Compute clusters run Spark applications this control plane did
				// not project; the owner label is what distinguishes ours.
				constants.OwnerServiceLabelKey: constants.MAOwnerSparkLabelValue,
				// MA controller manager runs multiple environments; only handle
				// jobs started by this environment.
				constants.JobControlPlaneEnvKey: r.env.RuntimeEnvironment,
			},
		},
		ResourceEventHandler: cache.ResourceEventHandlerFuncs{
			AddFunc:    r.sparkApplicationAddEventHandler,
			UpdateFunc: r.sparkApplicationUpdateEventHandler,
			DeleteFunc: r.sparkApplicationDeleteEventHandler,
		},
		Namespace: k8sengine.SparkLocalNamespace,
		ObjType:   &sparkv1beta2.SparkApplication{},
	}
}

// SparkApplication event handlers — own SparkJob.Status application fields.

// sparkApplicationAddEventHandler handles add events for SparkApplication resources.
func (r *Reconciler) sparkApplicationAddEventHandler(obj interface{}) {
	r.sparkApplicationEventHandler(obj)
}

// sparkApplicationUpdateEventHandler handles update events for SparkApplication resources.
func (r *Reconciler) sparkApplicationUpdateEventHandler(_, newObj interface{}) {
	// We re-inspect newObj even if already recorded to handle the case where we
	// missed the original event.
	r.sparkApplicationEventHandler(newObj)
}

// sparkApplicationEventHandler syncs SparkApplication status to the global SparkJob CR.
func (r *Reconciler) sparkApplicationEventHandler(obj interface{}) {
	local, ok := obj.(*sparkv1beta2.SparkApplication)
	if !ok {
		// Ignore events from ill-formed objects.
		return
	}
	log := r.logger.WithValues("spark_application", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the spark application")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	key := types.NamespacedName{Namespace: projectName, Name: local.Name}

	var sparkJob v2pb.SparkJob
	if err = r.Get(ctx, key, &sparkJob); err != nil {
		if utils.IsNotFoundError(err) {
			// The global SparkJob is gone: the ingester moved a frozen job to
			// metadata storage, or the job was deleted. The application of a job
			// with an outcome outlives it until the 24-hour TTL the projection
			// gives it, so the Spark Operator deletes it a day after it finishes;
			// the application of a job deleted before that was deleted by the
			// controller before the job went, unless it gave up on that
			// (handleDeletion). Either way the application's trailing events,
			// its delete included, are expected and there is nothing to update.
			log.V(1).Info("global spark job no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global spark job")
		return
	}

	if reason := sparkApplicationEventSkipReason(&sparkJob, local); reason != "" {
		log.V(1).Info("skipping status update for spark job", "reason", reason)
		return
	}

	// The Mapper is injected rather than constructed here so the control plane's
	// single configured k8sengine Mapper does the interpreting, and so tests can
	// substitute a mock.
	jobStatus, err := r.mapSparkApplicationStatus(local)
	if err != nil {
		log.Error(err, "could not map local spark application status")
		return
	}
	mapped := jobStatus.Spark

	// Cache re-sync gives Update events even when the application did not
	// change. Pre-check against a copy to avoid writing an unchanged object:
	// mergeSparkApplicationStatus reports whether it would alter anything, and
	// the copy keeps the probe from mutating the object we just read.
	// SparkJobStatus has no DeepCopy of its own, so the copy is taken from the
	// whole job.
	probe := sparkJob.DeepCopy().Status
	if !mergeSparkApplicationStatus(&probe, mapped, sparkJob.Generation) {
		return
	}

	log.Info("spark application event",
		"application_id", mapped.ApplicationId,
		"conditions", mapped.StatusConditions)

	if err = retry.OnError(retry.DefaultBackoff, isRetriableSparkJobWriteError, func() error {
		var current v2pb.SparkJob
		if getErr := r.Get(ctx, key, &current); getErr != nil {
			return getErr
		}
		// Re-check under the fresh read: the job may have been frozen, deleted,
		// terminated, finished or replaced between the pre-check and this attempt.
		if reason := sparkApplicationEventSkipReason(&current, local); reason != "" {
			log.V(1).Info("skipping status update for spark job", "reason", reason)
			return nil
		}
		if !mergeSparkApplicationStatus(&current.Status, mapped, current.Generation) {
			return nil
		}
		return r.Status().Update(ctx, &current)
	}); err != nil {
		logSparkJobWriteError(log, err, "could not update the global spark job status")
	}
}

// sparkApplicationDeleteEventHandler handles delete events for SparkApplication resources.
func (r *Reconciler) sparkApplicationDeleteEventHandler(obj interface{}) {
	// Recover the last-known SparkApplication from a tombstone if the informer
	// missed the final delete; the project namespace is only available from its
	// labels.
	var local *sparkv1beta2.SparkApplication
	switch v := obj.(type) {
	case *sparkv1beta2.SparkApplication:
		local = v
	case cache.DeletedFinalStateUnknown:
		r.logger.Info("received tombstone delete event for spark application", "key", v.Key)
		a, ok := v.Obj.(*sparkv1beta2.SparkApplication)
		if !ok {
			r.logger.Error(fmt.Errorf("could not extract SparkApplication from tombstone, unexpected object type %T", v.Obj), "skipping delete event")
			return
		}
		local = a
	default:
		r.logger.Error(fmt.Errorf("unexpected object type %T in delete handler", v), "skipping delete event")
		return
	}
	log := r.logger.WithValues("spark_application", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the spark application")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	key := types.NamespacedName{Namespace: projectName, Name: local.Name}

	var sparkJob v2pb.SparkJob
	if err = r.Get(ctx, key, &sparkJob); err != nil {
		if utils.IsNotFoundError(err) {
			log.V(1).Info("global spark job no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global spark job")
		return
	}

	// A job whose outcome is already recorded is done, and the deletion is
	// garbage collection, not a kill: the projection gives every application a
	// 24-hour TTL, so the Spark Operator deletes it a day after it finishes, long
	// after its outcome was recorded, and cluster teardown or an operator sweep
	// looks the same. Outcome and immutability are written by separate passes,
	// so the immutability check does not subsume the outcome check.
	//
	// Unlike the Ray controllers there is no Killing fork here either. What
	// keeps a deletion the control plane asked for from being taken for a
	// disappearance is the "termination was requested" skip reason, not a
	// recorded kill: handleTermination deletes the SparkApplication first and
	// records Killed only afterwards, so its delete event can arrive before the
	// kill is recorded, and the skip reason holds because Spec.Termination is
	// persisted before handleTermination acts on it. Likewise, the teardown of a
	// job that is itself being deleted (handleDeletion) is caught by the "job is
	// being deleted" skip reason. Whatever gets past these checks is a deletion
	// the control plane did not ask for.
	if reason := sparkApplicationEventSkipReason(&sparkJob, local); reason != "" {
		log.V(1).Info("skipping delete event for spark job", "reason", reason)
		return
	}

	// What the deletion means depends on the state the application was last
	// seen in, as the projection classifies it.
	//
	// A terminal state (COMPLETED, FAILED, SUBMISSION_FAILED) is the run's real
	// outcome, and is merged as the update event carrying it would have been.
	// That update may never have been written — a write that ran out of retries,
	// an API server outage — and the deletion is then the last event that
	// carries the outcome, which the 24-hour TTL makes the normal end of every
	// finished application. This holds for a tombstone too, whose last known
	// state may be older than the deletion: the projection sets no restart
	// policy, so these states are final, and an application last seen in one of
	// them ended in it.
	//
	// Any other state means the application was removed before it finished —
	// deleted by hand, or lost with its cluster. recordSparkApplicationGone
	// records that as a failure without a Killed condition: Killed=TRUE is what a
	// termination the user asked for records, and Uniflow takes it to mean "do
	// not retry".
	lastState := string(local.Status.AppState.State)
	jobStatus, err := r.mapSparkApplicationStatus(local)
	if err != nil {
		// With no projection the last state cannot be read as an outcome, so the
		// deletion is recorded as a disappearance.
		log.Error(err, "could not map the status of the deleted spark application")
	}
	recordedMessage := "spark application was deleted before it finished, recorded it as deleted"
	record := func(current *v2pb.SparkJob) bool {
		recordSparkApplicationGone(current, lastState)
		return true
	}
	if isTerminalSparkStatus(jobStatus) {
		recordedMessage = "spark application was deleted after it finished, recorded its outcome"
		record = func(current *v2pb.SparkJob) bool {
			return mergeSparkApplicationStatus(&current.Status, jobStatus.Spark, current.Generation)
		}
	}

	recorded := false
	if err = retry.OnError(retry.DefaultBackoff, isRetriableSparkJobWriteError, func() error {
		var current v2pb.SparkJob
		if getErr := r.Get(ctx, key, &current); getErr != nil {
			return getErr
		}
		// Re-check under the fresh read: an outcome recorded since the pre-check
		// — typically the update event for the same application, written just
		// before this deletion — must not be overwritten, and the job may have
		// been frozen, deleted, terminated or replaced meanwhile.
		if reason := sparkApplicationEventSkipReason(&current, local); reason != "" {
			log.V(1).Info("skipping delete event for spark job", "reason", reason)
			return nil
		}
		if !record(&current) {
			return nil
		}
		// No Status.State is written: v2pb.SparkJobStatus has no state enum, and
		// the conditions are the terminal record.
		if updateErr := r.Status().Update(ctx, &current); updateErr != nil {
			return updateErr
		}
		recorded = true
		return nil
	}); err != nil {
		logSparkJobWriteError(log, err, "failed to update status on external delete")
		return
	}
	if !recorded {
		// The fresh read gave a reason to leave the job alone. Whoever recorded
		// an outcome meanwhile has its own write reconcile the job, and the
		// controller freezes it then.
		return
	}
	log.Info(recordedMessage, "last_state", lastState)

	if err = r.markSparkJobImmutableIfOutcomeRecorded(ctx, &sparkJob); err != nil {
		logSparkJobWriteError(log, err, "failed to mark spark job immutable after external delete")
	}
}

// mapSparkApplicationStatus projects the application's status through the
// injected mapper. A mapper that returns no Spark status is reported as an
// error, so the caller always has one to log.
func (r *Reconciler) mapSparkApplicationStatus(local *sparkv1beta2.SparkApplication) (*matypes.JobStatus, error) {
	jobStatus, err := r.mapper.MapLocalJobStatusToGlobal(local)
	if err != nil {
		return nil, err
	}
	if jobStatus == nil || jobStatus.Spark == nil {
		return nil, fmt.Errorf("the mapper returned no spark status for spark application %q", local.Name)
	}
	return jobStatus, nil
}

// isRetriableSparkJobWriteError reports whether a failed write to a global
// SparkJob is worth another attempt: a conflict, which the next attempt's fresh
// read resolves, or a transient failure of the API server or of the connection
// to it (isTransientError).
//
// The watch's writes and the freeze retry on it with retry.DefaultBackoff,
// four attempts 10ms, 50ms and 250ms apart, rather than retry.DefaultRetry,
// whose five attempts are over in about 40ms. The client reads through the
// manager's cache, which can lag a write that just landed for longer than
// that, and an attempt that re-reads the same stale copy only conflicts again.
// A delete event is delivered once, so a write that gives up too early stays
// lost until the controller's probe finds the application gone, which happens
// only once the job's status has gone sparkStatusProbeInterval without a
// change, and later still while the job's cluster is held (holdCluster).
func isRetriableSparkJobWriteError(err error) bool {
	return apiErrors.IsConflict(err) || isTransientError(err)
}

// logSparkJobWriteError logs a write to the global SparkJob that failed for
// good. NotFound means the job was deleted between the handler's first read and
// its write, the same ordinary race as a NotFound on that first read, and is
// logged the same way.
func logSparkJobWriteError(logger logr.Logger, err error, msg string) {
	if utils.IsNotFoundError(err) {
		logger.V(1).Info("global spark job no longer exists, ignoring event")
		return
	}
	logger.Error(err, msg)
}

// markSparkJobImmutableIfOutcomeRecorded freezes a job whose outcome is
// recorded, so later reconciles and events leave it alone.
//
// Immutability is an annotation on the object, not a status field, so this is a
// plain Update and cannot be folded into a status write. The controller and
// the watch's delete handler share it, and both retry it on
// isRetriableSparkJobWriteError with retry.DefaultBackoff.
//
// The same Update removes sparkApplicationCleanupFinalizer: with the outcome
// recorded there is nothing left to stop, and the delete the ingester issues
// for a frozen job is never held up by it. A job whose deletion is pending
// goes as soon as that was its last finalizer.
func (r *Reconciler) markSparkJobImmutableIfOutcomeRecorded(ctx context.Context, sparkJob *v2pb.SparkJob) error {
	if err := retry.OnError(retry.DefaultBackoff, isRetriableSparkJobWriteError, func() error {
		latest := &v2pb.SparkJob{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: sparkJob.Namespace, Name: sparkJob.Name}, latest); err != nil {
			return err
		}
		if utils.IsImmutable(latest) || !isSparkJobOutcomeRecorded(latest) {
			return nil
		}
		utils.MarkImmutable(latest)
		ctrlutil.RemoveFinalizer(latest, sparkApplicationCleanupFinalizer)
		return r.Update(ctx, latest)
	}); err != nil {
		return fmt.Errorf("failed to mark spark job immutable: %w", err)
	}
	return nil
}

// sparkJobStatusUpdateSkipReason reports why the watch must not write to this
// global SparkJob, or the empty string when it may.
//
//   - Immutable: the job is frozen for good.
//   - Being deleted: the job has a DeletionTimestamp, or the API server's delete
//     path has marked it with the deleting annotation and left the delete to
//     the ingester (isSparkJobDeleting). The controller no longer launches or
//     probes such a job and tears its SparkApplication down, so the watch
//     leaves it alone as well.
//   - Termination requested: Reconcile hands the job to handleTermination,
//     which owns the outcome from then on — a late event from the application
//     the control plane is tearing down must not overwrite it.
func sparkJobStatusUpdateSkipReason(job *v2pb.SparkJob) string {
	switch {
	case utils.IsImmutable(job):
		return "job is immutable"
	case isSparkJobDeleting(job):
		return "job is being deleted"
	case job.Spec.GetTermination().GetType() != v2pb.TERMINATION_TYPE_INVALID:
		return "termination was requested"
	}
	return ""
}

// sparkApplicationEventSkipReason reports why the watch must not apply an
// event from app to this copy of job, or the empty string when it may. Every
// handler asks it twice: of the job it first reads, to skip the work, and of
// the fresh copy inside each write attempt, since any of the reasons can become
// true in between.
//
//   - sparkJobStatusUpdateSkipReason: frozen, being deleted, or a termination
//     was requested.
//   - The application is not the job's own (isSparkApplicationOf): another
//     job left it behind under the same name, or something other than the
//     control plane created it, and its state says nothing about this one.
//   - The job's outcome is recorded: a terminal state is final. The projection
//     sets no restart policy, so an application never leaves COMPLETED, FAILED
//     or SUBMISSION_FAILED for a state worth recording, and a recorded outcome —
//     including one the controller recorded for a creation that failed — is
//     never overwritten by a later event.
func sparkApplicationEventSkipReason(job *v2pb.SparkJob, app *sparkv1beta2.SparkApplication) string {
	if reason := sparkJobStatusUpdateSkipReason(job); reason != "" {
		return reason
	}
	switch {
	case !isSparkApplicationOf(app, job):
		return "spark application is not this spark job's own"
	case isSparkJobOutcomeRecorded(job):
		return "outcome already recorded"
	}
	return ""
}

// isSparkApplicationOf reports whether app is job's own SparkApplication: its
// job-UID label names job's UID, and that UID is not empty.
//
// Applications are named after their job in one namespace shared by every
// project on a member cluster, so the name alone cannot tell this job's
// application from another one under the same name. The projection labels
// every application it creates with its job's UID. An application whose label
// names a different UID was left by another job. One without the label, or
// with an empty one, was not created by the control plane for this job at all,
// but by hand or by another tool. The standalone Spark controller this one
// replaced is one such tool: it left unlabelled applications under their job's
// name, in the job's own namespace on the control plane's own cluster, so one
// of them holds a name here only for a job in the default namespace, when that
// cluster is also registered as a member. Neither kind is this job's, and a
// job without a UID owns none. The federated client decides ownership by the
// same rule when it creates, reads or deletes an application.
func isSparkApplicationOf(app *sparkv1beta2.SparkApplication, job *v2pb.SparkJob) bool {
	jobUID := string(job.GetUID())
	return jobUID != "" && app.GetLabels()[constants.JobUIDLabelKey] == jobUID
}

// isSparkJobOutcomeRecorded reports whether the job's final outcome is already
// written down.
//
// v2pb.SparkJobStatus has no state enum -- the control plane's state machine is
// the condition list -- so "terminal" is read off the conditions rather than
// off the Status.State field the Ray controllers consult. A Succeeded
// condition that has resolved either way is an outcome; an UNKNOWN one is an
// application still in flight.
func isSparkJobOutcomeRecorded(job *v2pb.SparkJob) bool {
	if isSparkJobKilled(job) {
		return true
	}
	succeeded := findSparkCondition(job.Status.StatusConditions, constants.SucceededCondition)
	return succeeded != nil &&
		(succeeded.Status == apipb.CONDITION_STATUS_TRUE || succeeded.Status == apipb.CONDITION_STATUS_FALSE)
}

// findSparkCondition looks a condition up by type without modifying the slice.
//
// jobsutils.GetCondition appends an UNKNOWN placeholder when the type is
// absent, which is the right behaviour for a caller about to write but the
// wrong one for a caller only asking a question.
func findSparkCondition(conditions []*apipb.Condition, conditionType string) *apipb.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}

// mergeSparkApplicationStatus folds the status projected from a
// SparkApplication event into the stored one, as mergeSparkStatus does, and
// reports whether anything changed.
//
// The one difference is ApplicationId and JobUrl: a projection that leaves
// either of them empty keeps the stored value instead of clearing it, just as
// the merge keeps a stored condition the projection omits. The operator fills
// them in once the driver is up, and once the job has reported a run's
// identifiers, an event that lacks them is no reason to forget them.
func mergeSparkApplicationStatus(dst, src *v2pb.SparkJobStatus, generation int64) bool {
	projected := *src
	if projected.ApplicationId == "" {
		projected.ApplicationId = dst.ApplicationId
	}
	if projected.JobUrl == "" {
		projected.JobUrl = dst.JobUrl
	}
	return mergeSparkStatus(dst, &projected, generation)
}

// mergeSparkStatus folds a freshly projected SparkJobStatus into the stored one
// and reports whether anything changed.
//
// It is an upsert, never a replacement. The condition slice is shared: the
// scheduler writes Enqueued and Scheduled onto it and handleTermination writes
// Killed, so replacing it would erase records this watch does not own. For the
// same reason a stored condition the projection omits is left alone --
// SparkAppRunning is only emitted while the application is RUNNING, and
// clearing it once the run ends would rewrite history rather than record it.
//
// The two traps in the condition helpers shape the loop. jobsutils.GetCondition
// appends an UNKNOWN condition when the type is absent, so the "did it change?"
// test uses the non-mutating lookup; otherwise a no-op event would grow the
// slice. jobsutils.UpdateCondition stamps LastUpdatedTimestamp unconditionally,
// so it is only reached for conditions that genuinely differ.
//
// Every condition written here gets generation — the SparkJob's
// metadata.generation as the caller read it — as its ObservedGeneration. It
// says which revision of the job the write was made against, not which spec
// the application runs: the application is created once, from the spec at
// launch, and later changes to the spec never reach it. The comparison leaves
// ObservedGeneration out, so a condition whose status, reason and message are
// unchanged keeps the generation it was last written at, even after the job's
// generation has moved on.
func mergeSparkStatus(dst, src *v2pb.SparkJobStatus, generation int64) bool {
	changed := false

	// ApplicationId and JobUrl are the mapper's to own; nothing else writes them.
	if dst.ApplicationId != src.ApplicationId {
		dst.ApplicationId = src.ApplicationId
		changed = true
	}
	if dst.JobUrl != src.JobUrl {
		dst.JobUrl = src.JobUrl
		changed = true
	}

	for _, projected := range src.StatusConditions {
		stored := findSparkCondition(dst.StatusConditions, projected.Type)
		if stored != nil &&
			stored.Status == projected.Status &&
			stored.Reason == projected.Reason &&
			stored.Message == projected.Message {
			continue
		}
		jobsutils.UpdateCondition(
			jobsutils.GetCondition(&dst.StatusConditions, projected.Type, generation),
			jobsutils.ConditionUpdateParams{
				Status:     projected.Status,
				Reason:     projected.Reason,
				Message:    projected.Message,
				Generation: generation,
			})
		changed = true
	}

	return changed
}
