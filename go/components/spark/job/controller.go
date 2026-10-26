// Package job implements the control-plane controller for SparkJob resources.
//
// Reconcile is the job's admission path: it puts a new SparkJob on the
// scheduler's queue, waits for the scheduler to assign it a member cluster, and
// then projects it there exactly once as a SparkApplication through the
// federated client. Until the launch it wakes every requeueAfter while the job
// waits for an assignment, and on the controller's backoff to retry a creation
// that failed for a transient reason, a failure it records on the job's
// Launched condition.
//
// From the moment the Launched condition is TRUE the federated watch in
// watcher.go owns the status of the running application, and Reconcile wakes
// for a launched job only to terminate it on request, to tear its
// SparkApplication down when the job itself is deleted, to freeze it once its
// outcome is recorded, and to read the application's status straight from the
// member cluster once the job's status has gone sparkStatusProbeInterval
// without a change. That probe is a backstop for watch events that never
// arrived, and it waits for a quiet status because a status that keeps
// changing shows the watch is delivering.
//
// A job that may own a SparkApplication carries a finalizer,
// sparkApplicationCleanupFinalizer, from just before its first CreateJob.
// Deleting such a job leaves it in place, marked with a DeletionTimestamp,
// until Reconcile has deleted the application or found nothing left to delete
// (handleDeletion). A delete that keeps failing leaves the job in place until
// sparkApplicationCleanupGiveUpAfter has passed since its deletion, and the
// first delete to fail after that gives up: Reconcile logs an Error naming the
// cluster and the application so that an operator can delete it by hand, and
// removes the finalizer anyway. The give-up is checked only after a delete
// fails, so a hold on the job's cluster, during which no delete is attempted,
// puts it off by the rest of each hold the job meets. A job whose outcome is
// recorded has nothing left to stop, so the write that freezes it removes the
// finalizer too.
//
// The controller runs maxConcurrentReconciles workers, ten, and each call it
// makes to a member cluster through the federated client may take up to
// remoteCallTimeout. A cluster that has stopped answering keeps a worker that
// long on each such call, so a call to it that runs out of time, like a probe
// that fails for any reason, holds every further call to that cluster for an
// interval (holdCluster). While the hold is on, the cluster's launches,
// terminations and deletions are requeued for the rest of the hold and its
// probes for another interval, without a call and without a write. Clusters
// that have stopped answering then cost at most one wave of calls per cluster
// per hold: on each of them, the calls already in flight when the first of
// them fails, no more than one on each of the ten workers, while the other
// workers go on with the jobs on other clusters. The queue's pre-flight check,
// which Enqueue runs against the job's target cluster on a deadline of its
// own, is not held.
package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/controllerutil"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	constants "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// requeueAfter defines the delay before retrying reconciliation.
	requeueAfter = 10 * time.Second

	// maxConcurrentReconciles is how many SparkJobs the controller reconciles
	// at once, the value the deployment and triggerrun controllers use.
	// controller-runtime never hands one SparkJob to two workers at the same
	// time, and everything Reconcile shares across jobs is safe for concurrent
	// use: the scheduler's queue, the registered-cluster cache, the federated
	// client and the clients it builds per cluster, and the cluster holds. A
	// call to a member cluster that has stopped answering keeps its own worker
	// waiting, for up to remoteCallTimeout, and the hold it then puts on its
	// cluster (holdCluster) keeps further calls to that cluster off the other
	// workers.
	maxConcurrentReconciles = 10

	// remoteCallTimeout bounds every call Reconcile makes to a member cluster's
	// API server through the federated client. Neither the reconcile context
	// nor the member-cluster clients carry a deadline of their own, so without
	// this a call to a member cluster that has stopped answering would keep its
	// worker for as long as the connection hangs. A call that runs out of time
	// holds every further call to its cluster for an interval (holdCluster), so
	// clusters that have stopped answering cost at most one wave of calls per
	// cluster per hold, no more than one on each of the ten workers, each
	// waiting out this deadline, rather than a call on every attempt of every
	// job assigned to them. The queue's pre-flight check runs under a deadline
	// of its own and is not held.
	remoteCallTimeout = 30 * time.Second

	// sparkStatusProbeInterval is how long a launched job with no recorded
	// outcome has to go without a status change before its SparkApplication's
	// status is read straight from the member cluster. That probe is a
	// level-triggered backstop for watch events lost across controllermgr
	// restarts, member-cluster outages or informer gaps: the federated watch is
	// edge-triggered, and an event it never delivered is not delivered later.
	// While the job's status keeps changing the watch is evidently delivering
	// and a probe would add nothing, so the probe waits for the status to go
	// quiet; a job whose status then stays put is probed once every interval.
	// It is also how long a hold on a member cluster lasts (holdCluster), how
	// old a rejection by the queue's pre-flight that could not ask its cluster
	// has to be before the job is enqueued again (rejectionRetryAfter), and how
	// old the record of a transient creation failure has to be before a failure
	// that says something else replaces it (markSparkApplicationCreationRetrying).
	sparkStatusProbeInterval = 5 * time.Minute

	// sparkApplicationCleanupFinalizer keeps a deleted SparkJob in place until
	// its SparkApplication has been torn down. Nothing in the member cluster
	// cascades from the SparkJob, since the application carries no owner
	// reference to it, and the application's TTL runs only once it has
	// finished, so a running application would otherwise outlive its job. A
	// SparkJob without a finalizer is gone in one step when it is deleted,
	// before Reconcile can see it being deleted.
	//
	// launch adds the finalizer before the job's first CreateJob, so only a
	// job that may own an application carries it, and nothing else adds it.
	// handleDeletion removes it once the teardown is done or given up, the
	// freeze (markSparkJobImmutableIfOutcomeRecorded) once the job's outcome is
	// recorded, and Reconcile's immutable arm when it finds one left on a frozen
	// job. The name follows the ingester's finalizer, api.IngesterFinalizer.
	sparkApplicationCleanupFinalizer = "michelangelo/SparkApplicationCleanup"

	// sparkApplicationCleanupGiveUpAfter is how long after a SparkJob's
	// deletion, by its DeletionTimestamp, handleDeletion keeps retrying a
	// delete of the job's SparkApplication that fails on a cluster the cache
	// resolves. The first delete to fail after that gives up and removes
	// sparkApplicationCleanupFinalizer anyway, so that a cluster which never
	// answers, or refuses the delete for good, cannot keep the job, and its
	// name, forever. No delete is attempted while the cluster is held
	// (holdCluster), so a hold puts the give-up off by the rest of the hold.
	sparkApplicationCleanupGiveUpAfter = time.Hour

	// Reasons the controller records on the SparkJob.
	reasonSparkApplicationCreated        = "SparkApplicationCreated"
	reasonSparkApplicationCreationFailed = "SparkApplicationCreationFailed"
	// reasonSparkApplicationCreationRetrying is the reason of the
	// Launched=UNKNOWN condition that records a transient failure to create the
	// job's SparkApplication, a creation the controller retries
	// (recordSparkApplicationCreationRetrying).
	reasonSparkApplicationCreationRetrying = "SparkApplicationCreationRetrying"
	// reasonSparkApplicationDeleted is the reason of the Succeeded=FALSE
	// condition recorded when a launched job's SparkApplication disappeared
	// before it reported an outcome (recordSparkApplicationGone).
	reasonSparkApplicationDeleted = "SparkApplicationDeleted"
)

// Reconciler manages the lifecycle of SparkJob custom resources.
//
// All fields are unexported. Exported struct fields become permanent public API surface —
// external packages can depend on them directly, making future removal a breaking change.
// Use NewReconciler() to construct instances.
//
// Key responsibilities:
//   - Enqueueing a new SparkJob with the scheduler, and re-enqueueing one that
//     is still waiting for an assignment after a controllermgr restart
//   - Creating the SparkApplication on the assigned member cluster, once
//   - Terminating that SparkApplication on request and recording the outcome
//   - Deleting it when the SparkJob itself is deleted. A finalizer added
//     before the launch, sparkApplicationCleanupFinalizer, keeps a deleted job
//     until then, or until sparkApplicationCleanupGiveUpAfter has passed since
//     the deletion with every delete failing
//   - Probing a launched job's SparkApplication for an outcome the watch missed
//   - Freezing a job whose outcome is recorded, so nothing touches it again
//
// The logger, the mapper and the metrics scope belong to the federated path,
// which the federated client and the cluster cache serve too: Register starts a
// watch on the SparkApplications this control plane projects into the member
// clusters, and the watch's event handlers (watcher.go) sync their status back
// onto the global SparkJob without waiting for a reconcile tick.
type Reconciler struct {
	client.Client                                       // Kubernetes client for local operations
	env             env.Context                         // Environment configuration context
	logger          logr.Logger                         // Logger for the federated watch's event handlers
	schedulerQueue  scheduler.JobQueue                  // Queue the scheduler assigns clusters from
	federatedClient jobsclient.FederatedClient          // Multi-cluster client the launch and the watch are built on
	clusterCache    jobscluster.RegisteredClustersCache // Member clusters to launch on and to watch
	mapper          matypes.Mapper                      // Projects local CR status back into the global CR
	metricsScope    tally.Scope                         // Metrics scope for the watch

	// clusterHolds records, for each member cluster a failed call has put on
	// hold, when that hold ends (holdCluster), and clusterHoldsMu guards it.
	// The map starts out nil and the first hold makes it, so a Reconciler built
	// without NewReconciler works too.
	clusterHoldsMu sync.Mutex
	clusterHolds   map[string]time.Time
}

// NewReconciler creates a new SparkJob reconciler with the required dependencies.
//
// mapper is injected rather than constructed here so tests can supply a mock and
// so the single k8sengine Mapper the rest of the control plane shares — with its
// configured log persistence and Kueue settings — is the one that interprets
// SparkApplication status.
func NewReconciler(
	logger logr.Logger,
	c client.Client,
	env env.Context,
	schedulerQueue scheduler.JobQueue,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobscluster.RegisteredClustersCache,
	mapper matypes.Mapper,
	metricsScope tally.Scope,
) *Reconciler {
	return &Reconciler{
		Client:          c,
		env:             env,
		logger:          logger,
		schedulerQueue:  schedulerQueue,
		federatedClient: federatedClient,
		clusterCache:    clusterCache,
		mapper:          mapper,
		metricsScope:    metricsScope,
	}
}

// Reconcile implements the Kubernetes reconciliation loop for SparkJob resources.
//
// The arms run in a fixed order, and every arm but the enqueue step ends the
// tick:
//  1. Immutable — the job is finished for good: do nothing but remove a
//     cleanup finalizer still on it
//  2. Outcome recorded — freeze the job, removing the cleanup finalizer in
//     the same write. The SparkApplication stays where it is: the projection
//     gives it a 24-hour TTL, so the Spark Operator deletes it a day after it
//     finishes
//  3. Being deleted — never enqueue or launch: delete the SparkApplication if
//     there may be one, then remove the cleanup finalizer
//  4. Termination requested — delete the SparkApplication and record the outcome
//  5. Enqueue, if the job is not on the scheduler's queue. Unless that fails,
//     the tick goes on from here to the assignment check: a job with no
//     cluster to launch on is requeued, and one with a cluster goes on to
//     arm 6 or 7
//  6. Launch, once the scheduler has assigned a cluster: add the cleanup
//     finalizer, then create the SparkApplication
//  7. Launched — probe the SparkApplication once the job's status has gone
//     sparkStatusProbeInterval without a change, and until then requeue for
//     the rest of that interval. While the job's cluster is held
//     (holdCluster), requeue for another interval instead
//
// Arms 3, 4 and 6 call the job's cluster only when it is not held: while it
// is, they requeue for the rest of the hold and write nothing.
//
// The outcome check deliberately precedes the termination check: a job that has
// already succeeded or failed keeps that outcome even if a termination request
// arrives afterwards.
//
// A write to the job that still conflicts once its retries are spent is not an
// error: another writer changed the job meanwhile, and that write's watch event
// reconciles the job again. Reconcile logs it at V(1) and requeues the job
// after requeueAfter, only as a backstop. Such a conflict is the API server's
// answer, a Conflict with a Kubernetes Status. A 409 whose body is not a Status
// reads as a Conflict too, but a proxy sent it, such as one in front of a
// member cluster answering a delete or the queue's pre-flight check, and it is
// returned like any other error. Every other error is returned with an empty
// Result. controller-runtime discards a RequeueAfter that comes with an error
// and retries on its rate limiter's backoff instead.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res, err := r.reconcile(ctx, req)
	if apiErrors.IsConflict(err) && !apiErrors.IsUnexpectedServerError(err) {
		log.FromContext(ctx).V(1).Info("a write to the spark job conflicted with a concurrent write, requeueing",
			"error", err.Error())
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return res, err
}

// reconcile is Reconcile but for the conflict it requeues: it runs the arms and
// returns whatever error ends the tick.
func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var sparkJob v2pb.SparkJob
	if err := r.Get(ctx, req.NamespacedName, &sparkJob); err != nil {
		if utils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if utils.IsImmutable(&sparkJob) {
		// The controller does nothing more for a frozen job, its teardown
		// included, so a cleanup finalizer still on one would only keep it from
		// being deleted. The freeze removes the finalizer in the same write;
		// this removes one left on a job frozen by some other writer.
		if err := r.removeSparkApplicationCleanupFinalizer(ctx, &sparkJob); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove the cleanup finalizer of frozen spark job %q: %w", req.NamespacedName, err)
		}
		return ctrl.Result{}, nil
	}

	if isSparkJobOutcomeRecorded(&sparkJob) {
		// The SparkApplication is never deleted here. The projection sets a
		// 24-hour TTL on it, so the Spark Operator deletes it 24 hours after
		// it finishes, and until then its driver logs and events outlive the
		// reconcile that observed the outcome.
		return ctrl.Result{}, r.markSparkJobImmutableIfOutcomeRecorded(ctx, &sparkJob)
	}

	if isSparkJobDeleting(&sparkJob) {
		return r.handleDeletion(ctx, logger, &sparkJob)
	}

	terminate, err := jobsutils.IsTerminationInfoSet(&sparkJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("check termination for %q: %w", req.NamespacedName, err)
	}
	if terminate {
		return r.handleTermination(ctx, logger, &sparkJob)
	}

	if err := r.enqueueIfRequired(ctx, &sparkJob, logger); err != nil {
		return ctrl.Result{}, err
	}

	// Not scheduled yet, or scheduled onto a cluster the cache does not know:
	// either way there is nowhere to launch on this tick.
	_, assignedCluster := r.assignedCluster(&sparkJob)
	if assignedCluster == nil {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if !isSparkJobLaunched(&sparkJob) {
		return r.launch(ctx, logger, &sparkJob, assignedCluster)
	}

	// Launched: the federated watch owns the status from here on, and this
	// path only checks, now and then, that the watch has not missed the end.
	return r.probeSparkApplication(ctx, logger, &sparkJob, assignedCluster)
}

// Register registers the SparkJob controller with the controller manager.
//
// This method configures the controller to watch SparkJob custom resources and
// trigger reconciliation when they are created, updated, or deleted.
//
// The controller runs maxConcurrentReconciles workers.
//
// It also hands the manager the federated watch on the projected
// SparkApplications, as a leader-only runnable. That watch runs outside the
// controller's work queue and writes to the same global CRs, so only the
// leader may run it: the manager starts it once this instance is elected and
// cancels its context when the manager shuts down.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	federatedWatcher := r.getFederatedWatcher()
	if err := mgr.Add(controllerutil.LeaderOnlyRunnable(func(ctx context.Context) error {
		// Start blocks until ctx is cancelled.
		federatedWatcher.Start(ctx)
		return nil
	})); err != nil {
		return fmt.Errorf("add the spark application watch to the manager: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.SparkJob{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconciles}).
		Complete(r)
}

// enqueueIfRequired puts the SparkJob on the scheduler's queue when it is not
// already there.
//
// The Enqueued condition tells three cases apart:
//   - Absent or UNKNOWN: a job seen for the first time. The condition is
//     written first and the job enqueued after. The scheduler reads the job
//     through the manager's cache, so it may still read a copy from before
//     the mark, but its assignment write is conditional on the
//     resourceVersion it read: a write from such a copy is refused with a
//     conflict and retried on a new read, so the scheduler cannot write the
//     job back without the mark.
//   - FALSE: the queue's pre-flight rejected the job, recorded why on the
//     condition and returned an error (the Kueue decorator does this for a
//     LocalQueue it cannot find or could not check, kueue.go). Each rejection
//     is a status write, and its watch event reconciles the job at once,
//     bypassing the error backoff, so a rejected job is retried only once its
//     rejection is old enough for its reason (rejectionRetryAfter), and TRUE
//     is written only after Enqueue succeeds rather than before it, where it
//     would erase the reason only for the decorator to write it back.
//   - TRUE but not scheduled: not redundant — the scheduler's queue lives in
//     memory, so every unscheduled job has to be put back on it after a
//     controllermgr restart.
func (r *Reconciler) enqueueIfRequired(ctx context.Context, job *v2pb.SparkJob, logger logr.Logger) error {
	schedulableJob := matypes.NewSchedulableJob(matypes.SchedulableJobParams{
		Name:       job.Name,
		Namespace:  job.Namespace,
		Generation: job.Generation,
		JobType:    matypes.SparkJob,
	})

	enqueued := findSparkCondition(job.Status.StatusConditions, constants.EnqueuedCondition)
	switch {
	case enqueued != nil && enqueued.Status == apipb.CONDITION_STATUS_FALSE:
		// Rejected by the queue's pre-flight: wait out the delay the
		// rejection's reason calls for, counted from the rejection, then try
		// again, and say so only once it worked. The caller requeues an
		// unscheduled job, so returning nil still comes back.
		retryAfter := rejectionRetryAfter(enqueued.Reason)
		if rejectedFor := time.Since(time.Unix(enqueued.LastUpdatedTimestamp, 0)); rejectedFor < retryAfter {
			logger.V(1).Info("scheduler queue rejected the spark job recently, not retrying yet",
				"reason", enqueued.Reason, "rejected_for", rejectedFor.String(), "retry_after", retryAfter.String())
			return nil
		}
		if err := r.schedulerQueue.Enqueue(ctx, schedulableJob); err != nil && !errors.Is(err, matypes.ErrJobAlreadyExists) {
			// Not returned: the pre-flight has recorded why on the Enqueued
			// condition, and an error would only add a backoff retry on top of
			// the requeue.
			logger.Info("scheduler queue rejected the spark job again", "error", err.Error())
			return nil
		}
		if _, err := r.updateStatus(ctx, job, markEnqueued); err != nil {
			return fmt.Errorf("status update err: %w", err)
		}
		logger.Info("enqueued spark job after an earlier rejection")
		return nil

	case enqueued == nil || enqueued.Status != apipb.CONDITION_STATUS_TRUE:
		// First time encountering the job, enqueue it
		if _, err := r.updateStatus(ctx, job, markEnqueued); err != nil {
			return fmt.Errorf("status update err: %w", err)
		}

		// Enqueue after status update to avoid conflicts
		if err := r.schedulerQueue.Enqueue(ctx, schedulableJob); err != nil {
			if errors.Is(err, matypes.ErrJobAlreadyExists) {
				// do not report error
				logger.V(1).Info("job already exists in the scheduler queue")
				return nil
			}
			return err
		}
		logger.Info("enqueued spark job")
		return nil
	}

	// In restart cases, enqueued but unscheduled jobs need to be placed back on the queue
	scheduledStatus := apipb.CONDITION_STATUS_UNKNOWN
	if scheduled := findSparkCondition(job.Status.StatusConditions, constants.ScheduledCondition); scheduled != nil {
		scheduledStatus = scheduled.Status
	}
	if scheduledStatus != apipb.CONDITION_STATUS_TRUE {
		if err := r.schedulerQueue.Enqueue(ctx, schedulableJob); err != nil {
			if errors.Is(err, matypes.ErrJobAlreadyExists) {
				logger.V(1).Info("spark job already exists in the scheduler queue")
				return nil
			}
			return err
		}

		logger.Info("enqueued spark job because not yet scheduled", "scheduled_condition_status", scheduledStatus)
	}
	return nil
}

// rejectionRetryAfter returns how old a rejection by the queue's pre-flight has
// to be, given its reason, before the job is put on the queue again. The age
// counts from the rejection's LastUpdatedTimestamp, which every rejection the
// pre-flight writes stamps afresh.
//
// KueueQueueCheckFailed means the pre-flight could not ask the target cluster
// about the job's LocalQueue at all: the check timed out, did not reach the
// cluster, or was refused or failed there. Each retry may hold a worker for as
// long as the check is allowed to take, and the jobs not pinned to a cluster
// may all target the same one, so retrying every requeueAfter while that
// cluster does not answer would make each pending SparkJob a periodic stall.
// Such a rejection is retried once every sparkStatusProbeInterval instead, as
// long as a hold on a cluster whose calls fail lasts (holdCluster), which does
// not cover this check. Any other reason, KueueQueueNotFound above all, is
// retried after requeueAfter: that check got its answer quickly, and a user who
// creates the missing LocalQueue sees the job enqueued soon after.
func rejectionRetryAfter(reason string) time.Duration {
	if reason == constants.KueueQueueCheckFailed {
		return sparkStatusProbeInterval
	}
	return requeueAfter
}

// markEnqueued is the updateStatus mutation that records the job as on the
// scheduler's queue.
func markEnqueued(current *v2pb.SparkJob) bool {
	cond := jobsutils.GetCondition(&current.Status.StatusConditions, constants.EnqueuedCondition, current.Generation)
	jobsutils.UpdateCondition(cond, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Reason:     constants.AddedToSchedulerQueue,
		Generation: current.Generation,
	})
	return true
}

// assignedCluster returns the name of the member cluster the scheduler assigned
// this SparkJob to, and that cluster as the registered-cluster cache knows it.
// The name is empty when the job is not scheduled or no assignment was written;
// the cluster is nil whenever the name is empty or the cache cannot resolve it.
// A miss need not mean that the cluster is no longer registered
// (teardownCluster), so the launch and the probe only wait for the cache to
// resolve the cluster.
func (r *Reconciler) assignedCluster(job *v2pb.SparkJob) (string, *v2pb.Cluster) {
	if !jobsutils.IsJobScheduled(job.Status.StatusConditions, job.Generation) {
		return "", nil
	}

	assignedName := job.Status.GetAssignment().GetCluster()
	if assignedName == "" {
		return "", nil
	}

	return assignedName, r.clusterCache.GetCluster(assignedName)
}

// teardownCluster is assignedCluster for handleTermination and handleDeletion,
// which delete the job's SparkApplication through the cluster it returns and
// take an assigned cluster it returns as nil to be deregistered. So that nil
// means just that, an assigned cluster missing from the registered-cluster
// cache is read from its Cluster object, and is nil only when that object is
// gone.
//
// The cache misses more than deregistered clusters. It lives in memory and
// starts empty: GetCluster (go/components/jobs/cluster/controller.go) reads a
// map the cluster controller fills as it reconciles each Cluster object, so
// for a while after the controllers start a registered cluster may not be in
// it yet. The cluster controller also adds a cluster only once its own
// finalizer is on the object, so a cluster whose finalizer write keeps failing
// stays out of the cache. A teardown that took such a miss for a deregistered
// cluster would delete nothing, and could leave the SparkApplication running
// with no job left to stop it. The Cluster object is what registers a cluster,
// so the teardown deletes through the object for as long as it exists, while
// it is being deleted too. The object is read through the controller's
// client, from the manager's cache of Cluster objects, which the cluster
// controller's watch keeps; a read waits for that cache to sync, so a NotFound
// means the object is gone.
//
// A read that fails other than with NotFound is returned as the error, with
// neither result set.
func (r *Reconciler) teardownCluster(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (string, *v2pb.Cluster, error) {
	assignedName, assignedCluster := r.assignedCluster(job)
	if assignedCluster != nil || assignedName == "" {
		return assignedName, assignedCluster, nil
	}

	var cluster v2pb.Cluster
	err := r.Get(ctx, types.NamespacedName{Namespace: constants.ClustersNamespace, Name: assignedName}, &cluster)
	switch {
	case err == nil:
		logger.Info("assigned cluster is not in the cluster cache but its Cluster object exists, tearing down through it",
			"cluster", assignedName)
		return assignedName, &cluster, nil
	case utils.IsNotFoundError(err):
		return assignedName, nil, nil
	}
	return "", nil, fmt.Errorf("read cluster %q: %w", assignedName, err)
}

// launch projects the SparkJob into its assigned member cluster as a
// SparkApplication and records the Launched condition.
//
// While the assigned cluster is held (holdCluster), launch calls nothing and
// writes nothing, and the job is requeued for the rest of the hold.
//
// Before the first CreateJob it adds sparkApplicationCleanupFinalizer, so that
// from then on deleting the job leaves it in place until handleDeletion has
// torn the application down (addSparkApplicationCleanupFinalizer). If the
// fresh copy of the job that write reads shows the job was frozen, deleted,
// terminated or finished since Reconcile read it, nothing is added or created,
// and the job is requeued for the arm that change calls for.
//
// A CreateJob error is handled by what it says:
//   - AlreadyExists: the job's own application is already there, left by an
//     earlier attempt whose result or Launched write was lost, so the job counts
//     as launched. An application under the same name that is not the job's
//     own is reported as jobsclient.ErrSparkApplicationConflict instead, never
//     AlreadyExists.
//   - Transient (isTransientError): the failure is recorded as
//     Launched=UNKNOWN (recordSparkApplicationCreationRetrying) and the error
//     is returned, so the create is retried on the controller's backoff.
//     Retrying after an ambiguous failure is safe: if the first create did
//     land, the retry finds the job's own application and gets AlreadyExists.
//     A create that ran out of time (isRemoteTimeout) also holds the cluster.
//   - Anything else — a validation or admission rejection, the name conflict, a
//     job the mapper cannot project — would fail the same way again, so it is
//     terminal: Launched=FALSE and Succeeded=FALSE are written, and the next
//     reconcile freezes the job. Both conditions carry the sanitized message
//     (sanitizeConditionMessage), the log the full error, and the error itself
//     is not returned: only a failure to record it is worth an error.
//
// Launched=TRUE, or the Launched=FALSE of a terminal failure, replaces the
// record of a transient one.
func (r *Reconciler) launch(
	ctx context.Context,
	logger logr.Logger,
	job *v2pb.SparkJob,
	assignedCluster *v2pb.Cluster,
) (ctrl.Result, error) {
	if res, held := r.waitForClusterHold(logger, assignedCluster.Name, "create"); held {
		return res, nil
	}

	skipReason, err := r.addSparkApplicationCleanupFinalizer(ctx, job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("add the cleanup finalizer to spark job %q/%q: %w", job.Namespace, job.Name, err)
	}
	if skipReason != "" {
		// The job's watch event for that change reconciles it too.
		logger.V(1).Info("not launching the spark job", "reason", skipReason)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	logger.Info("creating spark application via federated client", "assignedCluster", assignedCluster.Name)

	// jobClusterObject is nil: a SparkJob carries its driver and executor spec
	// itself, so unlike a RayJob it has no separate cluster CR to project.
	createCtx, cancel := context.WithTimeout(ctx, remoteCallTimeout)
	err = r.federatedClient.CreateJob(createCtx, job, nil, assignedCluster)
	cancel()

	switch {
	case err == nil:
	case apiErrors.IsAlreadyExists(err):
		logger.Info("spark application already exists, treating as launched")
	case isTransientError(err):
		if isRemoteTimeout(err) {
			r.holdClusterAfterTimeout(logger, assignedCluster.Name, err)
		}
		logger.Info("transient failure creating spark application, will retry", "error", err.Error())
		r.recordSparkApplicationCreationRetrying(ctx, logger, job, sanitizeConditionMessage(err, assignedCluster.Name))
		return ctrl.Result{}, fmt.Errorf("create spark application on cluster %q: %w", assignedCluster.Name, err)
	default:
		logger.Error(err, "failed to create spark application via federated client")

		message := sanitizeConditionMessage(err, assignedCluster.Name)
		if _, statusErr := r.updateStatus(ctx, job, func(current *v2pb.SparkJob) bool {
			// FALSE replaces the record of an earlier transient failure, which
			// would otherwise say the creation is still being retried.
			launched := jobsutils.GetCondition(&current.Status.StatusConditions, constants.LaunchedCondition, current.Generation)
			jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Generation: current.Generation,
				Reason:     reasonSparkApplicationCreationFailed,
				Message:    message,
			})
			succeeded := jobsutils.GetCondition(&current.Status.StatusConditions, constants.SucceededCondition, current.Generation)
			jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Generation: current.Generation,
				Reason:     reasonSparkApplicationCreationFailed,
				Message:    message,
			})
			return true
		}); statusErr != nil {
			logReconcileWriteError(logger, statusErr, "failed to update status after creation failure")
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if _, err := r.updateStatus(ctx, job, func(current *v2pb.SparkJob) bool {
		launched := jobsutils.GetCondition(&current.Status.StatusConditions, constants.LaunchedCondition, current.Generation)
		jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_TRUE,
			Generation: current.Generation,
			Reason:     reasonSparkApplicationCreated,
		})
		return true
	}); err != nil {
		logReconcileWriteError(logger, err, "failed to update launched condition")
		return ctrl.Result{}, err
	}

	logger.Info("spark application creation initiated")
	// The job is now a launched job with no outcome. The Launched write just
	// changed its status, so the first probe is due an interval from now.
	return ctrl.Result{RequeueAfter: sparkStatusProbeInterval}, nil
}

// recordSparkApplicationCreationRetrying records on the job a transient
// failure to create its SparkApplication, a creation launch returns for a retry
// on the controller's backoff: Launched=UNKNOWN with
// reasonSparkApplicationCreationRetrying and message, the failure as
// sanitizeConditionMessage renders it. Without it a job whose creation keeps
// failing for a cause that persists, a webhook whose calls keep failing or a
// proxy that keeps turning the request away, say, would wait Scheduled with
// nothing to say why.
//
// The record is never terminal: isSparkJobLaunched counts only TRUE, and
// Uniflow does not read Launched. It is written onto the freshest copy of the
// job (updateStatus), and only when markSparkApplicationCreationRetrying says
// it should be: each write's watch event reconciles the job at once, ahead of
// the backoff the returned error asks for. So an attempt that fails the same
// way as the one before writes nothing, and one that fails in a way that reads
// differently replaces the record only once the record is
// sparkStatusProbeInterval old. Until then the record shows the earlier
// failure, and its LastUpdatedTimestamp when that was recorded. Nothing is
// written either when sparkJobWriteSkipReason says the job must not be written
// to. A failed write is logged and not returned: the creation's own error is.
func (r *Reconciler) recordSparkApplicationCreationRetrying(
	ctx context.Context,
	logger logr.Logger,
	job *v2pb.SparkJob,
	message string,
) {
	if _, err := r.updateStatus(ctx, job, func(current *v2pb.SparkJob) bool {
		if reason := sparkJobWriteSkipReason(current); reason != "" {
			logger.V(1).Info("not recording the retried creation", "reason", reason)
			return false
		}
		return markSparkApplicationCreationRetrying(current, message, time.Now())
	}); err != nil {
		logReconcileWriteError(logger, err, "failed to record the retried creation of the spark application")
	}
}

// markSparkApplicationCreationRetrying is the updateStatus mutation of
// recordSparkApplicationCreationRetrying. It sets the job's Launched condition
// to UNKNOWN with reasonSparkApplicationCreationRetrying and message, and
// reports whether it did. A Launched condition that is TRUE is left alone: the
// application was created after all, by an earlier attempt that a stale copy
// of the job did not show.
//
// So is the record of an earlier transient failure that has the same message,
// and one with another message that is less than sparkStatusProbeInterval old
// at now, by its LastUpdatedTimestamp. A failure's message can change from one
// attempt to the next with nothing new to say: an error page a proxy sends in
// place of the API server's answer becomes part of the message, and it may
// quote a request ID that is new on every attempt. Rewritten on every attempt,
// the record's own watch events would reconcile the job again at once, every
// time, and the backoff between attempts would never take hold.
func markSparkApplicationCreationRetrying(current *v2pb.SparkJob, message string, now time.Time) bool {
	launched := jobsutils.GetCondition(&current.Status.StatusConditions, constants.LaunchedCondition, current.Generation)
	if launched.Status == apipb.CONDITION_STATUS_TRUE {
		return false
	}
	if launched.Status == apipb.CONDITION_STATUS_UNKNOWN && launched.Reason == reasonSparkApplicationCreationRetrying &&
		(launched.Message == message || now.Sub(time.Unix(launched.LastUpdatedTimestamp, 0)) < sparkStatusProbeInterval) {
		return false
	}
	jobsutils.UpdateCondition(launched, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_UNKNOWN,
		Generation: current.Generation,
		Reason:     reasonSparkApplicationCreationRetrying,
		Message:    message,
	})
	return true
}

// probeSparkApplication is the safety net for a launched job with no recorded
// outcome. It reads the SparkApplication's status straight from the member
// cluster, under remoteCallTimeout, and acts on what it finds:
//   - The application is absent (isSparkApplicationAbsent): it is gone without
//     having reported an outcome, so recordSparkApplicationGone records one.
//     The federated client reports an application under the same name that is
//     not the job's own as NotFound too, which is right: this job's own
//     application is gone either way.
//   - A terminal status (Succeeded TRUE or FALSE): applied with
//     mergeSparkApplicationStatus, exactly as the watch would have applied it,
//     so a stored ApplicationId or JobUrl the status leaves empty is kept.
//   - A status still in flight: nothing is written, and the next probe tries
//     again.
//   - Any other error, a 404 without a Kubernetes Status included
//     (isSparkApplicationAbsent says where one comes from): nothing is written
//     either, and calls to the job's cluster are held for an interval
//     (holdCluster).
//
// Only a job whose status has been quiet, with no condition written, for
// sparkStatusProbeInterval is probed (sparkStatusQuietWait). A change within
// the interval shows the watch is delivering, so the job is requeued instead
// for when the interval will have passed since that change. That keeps the
// probe off the reconcile the launch's own Launched write triggers and off the
// ones the watch's status writes trigger, and leaves one probe per interval for
// a job whose status stays put.
//
// A probe that fails for any reason other than the application's absence, a
// client for the cluster that could not be built included, holds every call to
// that cluster for sparkStatusProbeInterval (holdCluster, which says why). A
// job there whose probe falls due before the hold ends is requeued for another
// interval without a remote call, and the cluster's launches, terminations and
// deletions wait for the hold to end. A probe that finds the application gone
// or reads its status starts no hold.
//
// The write goes onto the freshest copy of the job and is skipped when the
// watch, the termination path or a deletion got there first
// (sparkJobWriteSkipReason). A job whose outcome the probe recorded is frozen by
// the next reconcile, like any other job with an outcome. Only a failed write
// is returned as an error.
func (r *Reconciler) probeSparkApplication(
	ctx context.Context,
	logger logr.Logger,
	job *v2pb.SparkJob,
	assignedCluster *v2pb.Cluster,
) (ctrl.Result, error) {
	next := ctrl.Result{RequeueAfter: sparkStatusProbeInterval}

	now := time.Now()
	if wait := sparkStatusQuietWait(job, now); wait > 0 {
		logger.V(1).Info("spark job status changed recently, not probing yet", "probe_in", wait.String())
		return ctrl.Result{RequeueAfter: wait}, nil
	}
	if until, held := r.clusterHeldUntil(assignedCluster.Name, now); held {
		logger.V(1).Info("calls to the spark job's cluster are on hold, not probing",
			"cluster", assignedCluster.Name, "until", until.Format(time.RFC3339))
		return next, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, remoteCallTimeout)
	status, err := r.federatedClient.GetJobStatus(probeCtx, job, assignedCluster)
	cancel()

	var record func(current *v2pb.SparkJob) bool
	switch {
	case isSparkApplicationAbsent(err):
		logger.Info("spark application is gone from its cluster, recording it as deleted", "cluster", assignedCluster.Name)
		record = func(current *v2pb.SparkJob) bool {
			recordSparkApplicationGone(current, lastObservedSparkState(current))
			return true
		}
	case err != nil:
		until := r.holdCluster(assignedCluster.Name, time.Now())
		logger.Info("could not probe the spark application, holding calls to its cluster",
			"cluster", assignedCluster.Name, "until", until.Format(time.RFC3339), "error", err.Error())
		return next, nil
	case !isTerminalSparkStatus(status):
		return next, nil
	default:
		logger.Info("probe found the spark application finished", "cluster", assignedCluster.Name)
		mapped := status.Spark
		record = func(current *v2pb.SparkJob) bool {
			return mergeSparkApplicationStatus(&current.Status, mapped, current.Generation)
		}
	}

	wrote, err := r.updateStatus(ctx, job, func(current *v2pb.SparkJob) bool {
		if reason := sparkJobWriteSkipReason(current); reason != "" {
			logger.V(1).Info("not recording the probe's result", "reason", reason)
			return false
		}
		return record(current)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("record the probed status of spark job %q/%q: %w", job.Namespace, job.Name, err)
	}
	if wrote {
		// An outcome is recorded now, and the next reconcile freezes the job.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return next, nil
}

// sparkStatusQuietWait returns how much longer job's status has to stay quiet,
// with no condition written, before a probe is due at now: zero once the newest
// LastUpdatedTimestamp among its conditions is sparkStatusProbeInterval old,
// and the rest of the interval until then. A timestamp later than now counts as
// written at now, so the wait is never longer than the interval.
func sparkStatusQuietWait(job *v2pb.SparkJob, now time.Time) time.Duration {
	var newest int64
	for _, condition := range job.Status.StatusConditions {
		newest = max(newest, condition.GetLastUpdatedTimestamp())
	}
	quietFor := max(now.Sub(time.Unix(newest, 0)), 0)
	if quietFor >= sparkStatusProbeInterval {
		return 0
	}
	return sparkStatusProbeInterval - quietFor
}

// holdCluster holds every call Reconcile makes to the named member cluster for
// sparkStatusProbeInterval from now, and returns when the hold ends. A hold
// already on is replaced, so the cluster stays held for an interval after the
// last failure that held it.
//
// Two kinds of failure start a hold: a probe that fails for any reason other
// than the application's absence (probeSparkApplication), and a create or a
// delete that ran out of time (isRemoteTimeout), the create in launch and the
// delete in deleteSparkApplication, which handleTermination and handleDeletion
// share. A create or a delete that failed fast, on a refused connection or a
// 503 say, starts none: it cost its worker next to nothing, and the
// controller's backoff retries it.
//
// While the hold is on, launch, handleTermination and handleDeletion requeue
// the job for the rest of the hold (waitForClusterHold), and the probe requeues
// it for another interval, all of them without a call to the cluster and
// without a write: nothing is recorded on the job, neither a Killed condition
// nor the record of a retried creation, and no cleanup finalizer is added or
// removed.
//
// The controller runs ten workers (maxConcurrentReconciles), and each call to
// a member cluster may take up to remoteCallTimeout. On a cluster that has
// stopped answering, such as one cut off by a network partition that drops its
// packets, every call waits out that deadline. Nothing else keeps jobs off such
// a cluster: when a cluster's health check fails, the cluster controller leaves
// its cached status as it was, so a cluster cut off from the control plane
// stays Ready in the registered-cluster cache, and the scheduler assigns a job
// that is not pinned to a cluster to any registered cluster, Ready or not.
// Without the hold, every attempt of every job assigned to such a cluster
// would keep a worker that long, and enough of them would leave the jobs on
// every other cluster waiting for a worker. With it, clusters that have
// stopped answering cost at most one wave of calls per cluster per hold: on
// each of them, the calls already in flight when the first of them fails, no
// more than one on each of the ten workers.
//
// The price is that a single failed probe, a fast one included, delays the
// cluster's launches, terminations and deletions as well, by up to one
// interval. A hold is never lifted early, by a call that succeeds or
// otherwise; it simply ends. The queue's pre-flight check, which Enqueue runs
// on a deadline of its own, is not held.
func (r *Reconciler) holdCluster(cluster string, now time.Time) time.Time {
	r.clusterHoldsMu.Lock()
	defer r.clusterHoldsMu.Unlock()

	if r.clusterHolds == nil {
		r.clusterHolds = make(map[string]time.Time)
	}
	until := now.Add(sparkStatusProbeInterval)
	r.clusterHolds[cluster] = until
	return until
}

// clusterHeldUntil returns when the last hold on the named member cluster
// ends, and whether that hold is still on at now. A hold that has ended is
// simply ignored until the next failure that holds the cluster replaces it.
func (r *Reconciler) clusterHeldUntil(cluster string, now time.Time) (time.Time, bool) {
	r.clusterHoldsMu.Lock()
	defer r.clusterHoldsMu.Unlock()

	until, ok := r.clusterHolds[cluster]
	return until, ok && now.Before(until)
}

// waitForClusterHold reports whether the named member cluster is held
// (holdCluster) and, when it is, logs that and returns the Result that
// requeues the job for the rest of the hold. operation names the call the job
// is waiting to make: "create", "terminate" or "delete".
func (r *Reconciler) waitForClusterHold(logger logr.Logger, cluster, operation string) (ctrl.Result, bool) {
	now := time.Now()
	until, held := r.clusterHeldUntil(cluster, now)
	if !held {
		return ctrl.Result{}, false
	}
	logger.V(1).Info("calls to the spark job's cluster are on hold, waiting for the hold to end",
		"cluster", cluster, "until", until.Format(time.RFC3339), "operation", operation)
	return ctrl.Result{RequeueAfter: until.Sub(now)}, true
}

// holdClusterAfterTimeout holds the named member cluster (holdCluster) after a
// call to it ran out of time (isRemoteTimeout), and logs the hold with the
// call's error.
func (r *Reconciler) holdClusterAfterTimeout(logger logr.Logger, cluster string, err error) {
	until := r.holdCluster(cluster, time.Now())
	logger.Info("the spark job's cluster did not answer in time, holding calls to it",
		"cluster", cluster, "until", until.Format(time.RFC3339), "error", err.Error())
}

// isTerminalSparkStatus reports whether a probed status records an outcome: a
// Succeeded condition that has resolved either way.
func isTerminalSparkStatus(status *matypes.JobStatus) bool {
	if status == nil || status.Spark == nil {
		return false
	}
	succeeded := findSparkCondition(status.Spark.StatusConditions, constants.SucceededCondition)
	return succeeded != nil &&
		(succeeded.Status == apipb.CONDITION_STATUS_TRUE || succeeded.Status == apipb.CONDITION_STATUS_FALSE)
}

// sparkJobWriteSkipReason reports why the launch and the probe must not write
// onto this copy of the job, or the empty string when they may: the watch's own
// reasons (sparkJobStatusUpdateSkipReason: frozen, being deleted, or a
// termination requested), and an outcome somebody else has recorded since
// Reconcile read the job. The probe asks it before recording its result, and
// the launch before adding the cleanup finalizer and before recording a
// transient creation failure.
func sparkJobWriteSkipReason(job *v2pb.SparkJob) string {
	if reason := sparkJobStatusUpdateSkipReason(job); reason != "" {
		return reason
	}
	if isSparkJobOutcomeRecorded(job) {
		return "outcome already recorded"
	}
	return ""
}

// lastObservedSparkState returns the SparkApplication state the watch last
// recorded on the job — the reason of a Succeeded condition that is still
// UNKNOWN, as the projection writes it — or the empty string if it recorded none.
func lastObservedSparkState(job *v2pb.SparkJob) string {
	succeeded := findSparkCondition(job.Status.StatusConditions, constants.SucceededCondition)
	if succeeded == nil || succeeded.Status != apipb.CONDITION_STATUS_UNKNOWN {
		return ""
	}
	return succeeded.Reason
}

// recordSparkApplicationGone records on job that its SparkApplication
// disappeared from the member cluster before reporting an outcome:
// Succeeded=FALSE with reasonSparkApplicationDeleted, and a message naming the
// assigned cluster and, when lastState is not empty, the last state the
// application was seen in.
//
// It records no Killed condition, on purpose. Killed=TRUE is what a
// user-requested termination records (handleTermination, including its
// status-only kills), and Uniflow reads it first and takes it to mean "do not
// retry" (python/michelangelo/uniflow/commons.star). An application that
// vanished — deleted by hand, lost with its cluster or during an outage — is a
// failure Uniflow may retry.
//
// It only changes job in memory, so any writer can use it inside its own
// read-check-write loop on the freshest copy of the job: the probe does,
// through updateStatus, and the watch's delete handler can, passing the state
// of the application it saw deleted.
func recordSparkApplicationGone(job *v2pb.SparkJob, lastState string) {
	message := fmt.Sprintf("the SparkApplication on cluster %q was deleted before it reported an outcome",
		job.Status.GetAssignment().GetCluster())
	if lastState != "" {
		message = fmt.Sprintf("%s; last observed state: %s", message, lastState)
	}

	succeeded := jobsutils.GetCondition(&job.Status.StatusConditions, constants.SucceededCondition, job.Generation)
	jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_FALSE,
		Reason:     reasonSparkApplicationDeleted,
		Generation: job.Generation,
		Message:    message,
	})
}

// updateStatus applies mutate to the freshest copy of the SparkJob and writes
// the status subresource, retrying the whole read-modify-write on conflict.
//
// mutate reports whether the fresh copy needs writing at all. A mutation that
// re-checks a guard against the fresh object — an outcome recorded since the
// caller's snapshot, say — returns false, and nothing is written. updateStatus
// reports whether it wrote.
//
// On success the caller's copy is replaced by the freshest object updateStatus
// saw, the one it wrote or, when mutate declined, the one it read, so a second
// status write in the same reconcile does not conflict with the first on a
// stale resourceVersion.
func (r *Reconciler) updateStatus(ctx context.Context, job *v2pb.SparkJob, mutate func(*v2pb.SparkJob) bool) (bool, error) {
	return r.updateLatest(ctx, job, mutate, func(current *v2pb.SparkJob) error {
		return r.Status().Update(ctx, current)
	})
}

// updateMetadata is updateStatus for the job's metadata, its finalizers among
// it: the write is a plain Update, which leaves the status as the API server
// has it. When it takes the last finalizer off a job with a DeletionTimestamp,
// the API server deletes the job.
func (r *Reconciler) updateMetadata(ctx context.Context, job *v2pb.SparkJob, mutate func(*v2pb.SparkJob) bool) (bool, error) {
	return r.updateLatest(ctx, job, mutate, func(current *v2pb.SparkJob) error {
		return r.Update(ctx, current)
	})
}

// updateLatest is the read-modify-write updateStatus and updateMetadata share:
// it reads the freshest copy of the job, applies mutate, has write store it
// when mutate asks for that, retries the whole of it on conflict, and leaves
// the caller's copy current.
//
// It retries with retry.DefaultBackoff, four attempts 10ms, 50ms and 250ms
// apart, for the reason the watch's writes do (isRetriableSparkJobWriteError):
// the read goes through the manager's cache, which can lag a write that just
// landed for longer than retry.DefaultRetry's five attempts take. A conflict
// that outlasts the four is returned, and Reconcile requeues the job for it.
func (r *Reconciler) updateLatest(
	ctx context.Context,
	job *v2pb.SparkJob,
	mutate func(*v2pb.SparkJob) bool,
	write func(*v2pb.SparkJob) error,
) (bool, error) {
	key := client.ObjectKeyFromObject(job)

	var (
		latest v2pb.SparkJob
		wrote  bool
	)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var current v2pb.SparkJob
		if err := r.Get(ctx, key, &current); err != nil {
			return err
		}
		wrote = mutate(&current)
		if wrote {
			if err := write(&current); err != nil {
				return err
			}
		}
		latest = current
		return nil
	}); err != nil {
		return false, err
	}

	*job = latest
	return wrote, nil
}

// logReconcileWriteError logs a write to the job that failed during a
// reconcile, with msg and keysAndValues. A conflict is logged at V(1):
// Reconcile requeues the job for it rather than returning an error, since
// another writer changed the job meanwhile and its write reconciles the job
// again. Any other failure is logged as an Error.
func logReconcileWriteError(logger logr.Logger, err error, msg string, keysAndValues ...any) {
	if apiErrors.IsConflict(err) {
		logger.V(1).WithValues(keysAndValues...).Info(msg, "error", err.Error())
		return
	}
	logger.Error(err, msg, keysAndValues...)
}

// addSparkApplicationCleanupFinalizer puts sparkApplicationCleanupFinalizer on
// the job, onto its freshest copy (updateMetadata), and leaves job current.
// Nothing is written when the job already carries it.
//
// It returns a reason, and adds nothing, when the fresh copy shows the job must
// not be launched after all: it was frozen, is being deleted, had a termination
// requested or recorded an outcome since Reconcile read it
// (sparkJobWriteSkipReason). The API server would refuse a new finalizer on a
// job whose deletion is already pending anyway.
func (r *Reconciler) addSparkApplicationCleanupFinalizer(ctx context.Context, job *v2pb.SparkJob) (string, error) {
	if ctrlutil.ContainsFinalizer(job, sparkApplicationCleanupFinalizer) {
		return "", nil
	}
	var skipReason string
	if _, err := r.updateMetadata(ctx, job, func(current *v2pb.SparkJob) bool {
		if skipReason = sparkJobWriteSkipReason(current); skipReason != "" {
			return false
		}
		return ctrlutil.AddFinalizer(current, sparkApplicationCleanupFinalizer)
	}); err != nil {
		return "", err
	}
	return skipReason, nil
}

// removeSparkApplicationCleanupFinalizer takes sparkApplicationCleanupFinalizer
// off the job, off its freshest copy (updateMetadata), and leaves job current.
// Nothing is written when the job does not carry it, and a job that is gone
// already has nothing left to remove. Once the last finalizer is off a job
// whose deletion is pending, the API server deletes it.
func (r *Reconciler) removeSparkApplicationCleanupFinalizer(ctx context.Context, job *v2pb.SparkJob) error {
	if !ctrlutil.ContainsFinalizer(job, sparkApplicationCleanupFinalizer) {
		return nil
	}
	_, err := r.updateMetadata(ctx, job, func(current *v2pb.SparkJob) bool {
		return ctrlutil.RemoveFinalizer(current, sparkApplicationCleanupFinalizer)
	})
	return client.IgnoreNotFound(err)
}

// isSparkJobKilled reports whether the SparkJob has already reached the killed
// terminal state, so termination handling is idempotent across reconciles.
func isSparkJobKilled(job *v2pb.SparkJob) bool {
	for _, cond := range job.Status.StatusConditions {
		if cond.Type == constants.KilledCondition && cond.Status == apipb.CONDITION_STATUS_TRUE {
			return true
		}
	}
	return false
}

// isSparkJobDeleting reports whether the SparkJob is on its way out: its
// DeletionTimestamp is set, or the API server's delete path has marked it with
// the deleting annotation and left the actual delete to the ingester
// (go/components/ingester/controller.go isDeletingAnnotationSet reads the same
// annotation).
func isSparkJobDeleting(job *v2pb.SparkJob) bool {
	return job.GetDeletionTimestamp() != nil || utils.IsDeleting(job)
}

// getSuccessFromTerminateSpec derives the Succeeded condition status from the
// requested termination type: an explicit SUCCEEDED termination resolves to TRUE,
// anything else (FAILED, etc.) to FALSE.
func getSuccessFromTerminateSpec(job *v2pb.SparkJob) apipb.ConditionStatus {
	if job.Spec.GetTermination().GetType() == v2pb.TERMINATION_TYPE_SUCCEEDED {
		return apipb.CONDITION_STATUS_TRUE
	}
	return apipb.CONDITION_STATUS_FALSE
}

// deleteSparkApplication asks the member cluster to delete the job's
// SparkApplication, under remoteCallTimeout. An application that is already
// absent (isSparkApplicationAbsent) is not an error, while a 404 without a
// Kubernetes Status is, so the delete is retried. The federated client also
// returns nil, deleting nothing, when the application under the job's name is
// not the job's own. A delete that ran out of time (isRemoteTimeout) holds the
// cluster (holdCluster) before its error is returned.
func (r *Reconciler) deleteSparkApplication(
	ctx context.Context,
	logger logr.Logger,
	job *v2pb.SparkJob,
	cluster *v2pb.Cluster,
) error {
	deleteCtx, cancel := context.WithTimeout(ctx, remoteCallTimeout)
	defer cancel()

	err := r.federatedClient.DeleteJob(deleteCtx, job, cluster)
	switch {
	case err == nil, isSparkApplicationAbsent(err):
		return nil
	case isRemoteTimeout(err):
		r.holdClusterAfterTimeout(logger, cluster.Name, err)
	}
	return err
}

// handleDeletion is the arm for a SparkJob that is being deleted
// (isSparkJobDeleting). Such a job is never enqueued or launched.
//
// If it was scheduled to a cluster teardownCluster resolves, its
// SparkApplication is deleted whether or not Launched is visible, as in
// handleTermination: nothing in the member cluster cascades from the SparkJob,
// since the SparkApplication carries no owner reference to it, and the TTL
// only runs once the application has finished, so a running application would
// otherwise outlive its job. A cluster the cache cannot resolve is resolved
// from its Cluster object instead. While the cluster is held (holdCluster),
// nothing is deleted or written, and the job is requeued for the rest of the
// hold.
//
// A job that launch put sparkApplicationCleanupFinalizer on is held by it until
// this teardown is over, and the finalizer is removed once there is nothing
// left to do:
//   - The delete succeeded, found the application already absent
//     (isSparkApplicationAbsent), or left alone an application under the job's
//     name that is not the job's own, which the federated client does not
//     delete.
//   - The assigned cluster is missing from the cache and its Cluster object
//     is gone, so it is deregistered, or the job was never assigned one.
//   - The delete keeps failing and the job's DeletionTimestamp is more than
//     sparkApplicationCleanupGiveUpAfter old (sparkApplicationCleanupOverdue).
//     The controller gives up then: it logs an Error naming the cluster, the
//     application and the last error, so that an operator can delete the
//     application by hand, and removes the finalizer. The give-up is checked
//     only after a delete attempt fails, and none is attempted while the
//     cluster is held, so each hold the job meets puts the give-up off by the
//     rest of that hold, no more than one interval.
//
// Any other failed delete keeps the finalizer and is returned, so it is retried
// on the controller's backoff, and so is a failed read of the Cluster object,
// which comes before any delete. A job marked only with the deleting annotation
// has no DeletionTimestamp yet, so the finalizer holds up nothing and the
// give-up never applies: the same teardown runs, and the finalizer is removed
// once it is done. Nothing is written to the job's status.
func (r *Reconciler) handleDeletion(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (ctrl.Result, error) {
	assignedName, assignedCluster, err := r.teardownCluster(ctx, logger, job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("resolve the cluster of deleted spark job %q/%q: %w", job.Namespace, job.Name, err)
	}
	switch {
	case assignedCluster != nil:
		if res, held := r.waitForClusterHold(logger, assignedCluster.Name, "delete"); held {
			return res, nil
		}
		err = r.deleteSparkApplication(ctx, logger, job, assignedCluster)
		switch {
		case err == nil:
			logger.Info("SparkJob is being deleted, its SparkApplication was deleted", "cluster", assignedCluster.Name)
		case sparkApplicationCleanupOverdue(job, time.Now()):
			logger.Error(err, "gave up deleting the SparkApplication of a deleted SparkJob, which may still be running: delete it by hand",
				"cluster", assignedCluster.Name,
				"spark_application", k8sengine.SparkLocalNamespace+"/"+job.Name,
				"deleted_at", job.GetDeletionTimestamp().UTC().Format(time.RFC3339))
		default:
			logger.Error(err, "failed to delete the SparkApplication of a SparkJob being deleted",
				"operation", "delete_job",
				"namespace", job.Namespace,
				"name", job.Name,
				"cluster", assignedCluster.Name)
			return ctrl.Result{}, fmt.Errorf("delete spark application of deleted spark job %q/%q: %w", job.Namespace, job.Name, err)
		}
	default:
		logger.Info("SparkJob is being deleted, it has no SparkApplication the control plane can reach",
			"cluster", assignedName)
	}

	if err := r.removeSparkApplicationCleanupFinalizer(ctx, job); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove the cleanup finalizer of deleted spark job %q/%q: %w", job.Namespace, job.Name, err)
	}
	return ctrl.Result{}, nil
}

// sparkApplicationCleanupOverdue reports whether job's deletion, by its
// DeletionTimestamp, is more than sparkApplicationCleanupGiveUpAfter old at
// now, after which handleDeletion stops retrying a delete that keeps failing.
// A job marked only with the deleting annotation has no DeletionTimestamp and
// is never overdue.
func sparkApplicationCleanupOverdue(job *v2pb.SparkJob, now time.Time) bool {
	deleted := job.GetDeletionTimestamp()
	return deleted != nil && now.Sub(deleted.Time) > sparkApplicationCleanupGiveUpAfter
}

// handleTermination deletes the projected SparkApplication and drives the
// SparkJob to a terminal state, marking it immutable.
//
// What happens in the member cluster depends on how far the job got:
//   - Scheduled to a cluster the cache resolves, or to one missing from the
//     cache whose Cluster object exists (teardownCluster): the
//     SparkApplication is deleted whether or not Launched is visible. That
//     flag can lag the application — a cached read just after the launch, or
//     a Launched write that failed after the create succeeded — and deleting
//     costs one call when there is nothing there: an application already
//     absent (isSparkApplicationAbsent) counts as success. Any other delete
//     error, a 404 without a Kubernetes Status included, leaves the status
//     untouched and is returned, so the kill is retried rather than recorded
//     as done. While the cluster is held (holdCluster), nothing is deleted or
//     recorded, and the job is requeued for the rest of the hold.
//   - Scheduled to a cluster missing from the cache whose Cluster object is
//     gone: the cluster is deregistered, and the kill is recorded without a
//     delete, as the Ray cluster controller does, with a message saying the
//     SparkApplication could not be deleted. A read of the Cluster object
//     that fails records nothing and is returned, so the kill is retried.
//   - Never scheduled: nothing can exist remotely, so the kill is status only.
//
// Termination owns the job's outcome once it is requested: from then on
// neither the watch nor the probe records anything
// (sparkJobStatusUpdateSkipReason, which sparkJobWriteSkipReason includes), so
// an application that finishes while its delete is in flight is recorded as
// Killed all the same. An outcome recorded before the request is kept instead,
// because Reconcile freezes a job with an outcome before it looks for a
// termination. The kill is written onto the freshest copy of the job, and the
// write's check for an outcome recorded since Reconcile read the job is a
// defensive guard: nothing in this package records one once a termination is
// requested. The job is frozen either way.
func (r *Reconciler) handleTermination(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (ctrl.Result, error) {
	// A job already recorded as killed has nothing left to delete or record,
	// but the freeze below is a write of its own that may not have happened, so
	// the job still goes through it. Reconcile's outcome arm catches a killed
	// job first; this keeps the function correct on its own.
	if !isSparkJobKilled(job) {
		assignedName, assignedCluster, err := r.teardownCluster(ctx, logger, job)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("resolve the cluster of terminated spark job %q/%q: %w", job.Namespace, job.Name, err)
		}

		var message string
		switch {
		case assignedCluster != nil:
			if res, held := r.waitForClusterHold(logger, assignedCluster.Name, "terminate"); held {
				return res, nil
			}
			if err = r.deleteSparkApplication(ctx, logger, job, assignedCluster); err != nil {
				logger.Error(err, "failed to terminate SparkApplication",
					"operation", "terminate_job",
					"namespace", job.Namespace,
					"name", job.Name)
				return ctrl.Result{}, fmt.Errorf("terminate spark job %q/%q: %w", job.Namespace, job.Name, err)
			}
			message = "Spark job killed"
			if reason := job.Spec.GetTermination().GetReason(); reason != "" {
				message = reason
			}
		case assignedName != "":
			message = fmt.Sprintf("the SparkApplication on cluster %q could not be deleted because that cluster is not registered with the control plane", assignedName)
		default:
			message = constants.KilledMessageJobNotLaunched
		}

		// Drive the SparkJob to its terminal state using the shared condition
		// helpers: set Killed and the Succeeded condition derived from the
		// termination type, so consumers have a definite outcome for the
		// terminated job.
		wrote, err := r.updateStatus(ctx, job, func(current *v2pb.SparkJob) bool {
			// Defensive: the watch and the probe record no outcome once a
			// termination is requested, so this finds one only if something
			// else wrote it.
			if isSparkJobOutcomeRecorded(current) {
				return false
			}
			killed := jobsutils.GetCondition(&current.Status.StatusConditions, constants.KilledCondition, current.Generation)
			jobsutils.UpdateCondition(killed, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: current.Generation,
				Reason:     constants.SparkAppKilled,
				Message:    message,
			})
			succeeded := jobsutils.GetCondition(&current.Status.StatusConditions, constants.SucceededCondition, current.Generation)
			jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
				Status:     getSuccessFromTerminateSpec(current),
				Generation: current.Generation,
				Reason:     current.Spec.GetTermination().GetReason(),
			})
			return true
		})
		if err != nil {
			logReconcileWriteError(logger, err, "failed to update SparkJob status",
				"operation", "update_status",
				"namespace", job.Namespace,
				"name", job.Name)
			return ctrl.Result{}, fmt.Errorf("update spark job status for %q/%q: %w", job.Namespace, job.Name, err)
		}
		if !wrote {
			logger.Info("SparkJob recorded an outcome while it was being terminated, keeping it",
				"namespace", job.Namespace,
				"name", job.Name)
		}
	}

	// Mark the SparkJob immutable so it cannot transition further once killed.
	// The outcome is persisted above, so this is the shared helper's precondition.
	if err := r.markSparkJobImmutableIfOutcomeRecorded(ctx, job); err != nil {
		logReconcileWriteError(logger, err, "failed to update SparkJob",
			"operation", "update",
			"namespace", job.Namespace,
			"name", job.Name)
		return ctrl.Result{}, err
	}

	logger.Info("SparkJob terminated", "name", job.Name, "namespace", job.Namespace)
	return ctrl.Result{}, nil
}

// isSparkJobLaunched reports whether the control plane has already created the
// SparkApplication on the assigned member cluster.
func isSparkJobLaunched(job *v2pb.SparkJob) bool {
	launched := findSparkCondition(job.Status.StatusConditions, constants.LaunchedCondition)
	return launched != nil && launched.Status == apipb.CONDITION_STATUS_TRUE
}
