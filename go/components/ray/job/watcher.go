package job

import (
	"context"
	"fmt"
	"time"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/watch"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	_eventHandlerTimeout = 10 * time.Second
	// _jobStatusUnknown is the JobStatus of a job whose KubeRay RayJob is gone
	// from the compute cluster. It is not a status KubeRay reports.
	_jobStatusUnknown = "Unknown"
)

// isRetriableWriteError reports whether a handler's status write is worth
// retrying: a conflict with a concurrent writer, or a transient API server
// error -- throttling, an unavailable or timed-out server, or an etcd timeout.
// The delete path in particular gets a single event, so a dropped write is left
// to the reconciler's slow safety net.
func isRetriableWriteError(err error) bool {
	return apiErrors.IsConflict(err) ||
		apiErrors.IsTooManyRequests(err) ||
		apiErrors.IsServiceUnavailable(err) ||
		apiErrors.IsServerTimeout(err) ||
		apiErrors.IsTimeout(err) ||
		jobsutils.IsRetriableError(err)
}

// getFederatedWatcher creates a federated watcher for the RayJob controller.
//
// It watches a single resource on every ready compute cluster: the KubeRay
// RayJob, to propagate the job execution status (State/JobStatus/Message) onto
// the global v2pb.RayJob. This is the event-driven replacement for the previous
// GetJobStatus polling (TODO #605).
//
// Cluster readiness, HeadNode and PodErrors live on v2pb.RayCluster and are
// owned by the RayCluster controller's watcher — this controller never writes
// them, so there is no Pod or KubeRay-RayCluster watch here.
func (r *Reconciler) getFederatedWatcher() watch.FederatedWatcher {
	return watch.NewFederatedWatcher(watch.FederatedWatcherParams{
		ClusterCache:    r.clusterCache,
		FederatedClient: r.federatedClient,
		Logger:          r.logger.WithValues("resource", "rayjob"),
		WatcherParams: []*jobsclient.WatcherParams{
			{
				ResourceName: constants.KubeRayJobResource,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.OwnerServiceLabelKey: constants.MAOwnerServiceLabelValue,
						// MA controller manager runs multiple environments; only handle
						// jobs started by this environment.
						constants.JobControlPlaneEnvKey: r.env.RuntimeEnvironment,
					},
				},
				ResourceEventHandler: cache.ResourceEventHandlerFuncs{
					AddFunc:    r.rayJobAddEventHandler,
					UpdateFunc: r.rayJobUpdateEventHandler,
					DeleteFunc: r.rayJobDeleteEventHandler,
				},
				Namespace: k8sengine.RayLocalNamespace,
				ObjType:   &rayv1.RayJob{},
			},
		},
		Scope: r.metricsScope.SubScope("rayjob"),
	})
}

// KubeRay RayJob event handlers — own RayJob.Status execution fields.

// rayJobAddEventHandler handles add events for KubeRay RayJob resources.
func (r *Reconciler) rayJobAddEventHandler(obj interface{}) {
	r.rayJobEventHandler(obj)
}

// rayJobUpdateEventHandler handles update events for KubeRay RayJob resources.
func (r *Reconciler) rayJobUpdateEventHandler(_, newObj interface{}) {
	// We re-inspect newObj even if already recorded to handle the case where we
	// missed the original event.
	r.rayJobEventHandler(newObj)
}

// rayJobEventHandler syncs KubeRay RayJob execution status to the global RayJob CR.
func (r *Reconciler) rayJobEventHandler(obj interface{}) {
	local, ok := obj.(*rayv1.RayJob)
	if !ok {
		// Ignore events from ill-formed objects.
		return
	}
	log := r.logger.WithValues("ray_job", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the ray job")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayJob v2pb.RayJob
	if err = r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &rayJob); err != nil {
		if utils.IsNotFoundError(err) {
			// The global RayJob is gone: the ingester moved a terminal job to metadata
			// storage, or it was deleted outright. The KubeRay object outlives it until
			// owner GC removes it with its KubeRay RayCluster (KubeRay skips TTL cleanup
			// for jobs that target an existing cluster), so its trailing events are
			// expected and there is nothing to update.
			log.V(1).Info("global ray job no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global ray job")
		return
	}

	// Skip updates if the global RayJob is immutable, being deleted, or already
	// terminal: a terminal state is final, even before the job is frozen.
	if utils.IsImmutable(&rayJob) {
		log.V(1).Info("skipping status update for immutable job")
		return
	}
	if rayJob.GetDeletionTimestamp() != nil {
		log.V(1).Info("skipping status update for job being deleted")
		return
	}
	if isTerminalRayJobState(rayJob.Status.State) {
		log.V(1).Info("skipping status update for job that already reached a terminal state", "state", rayJob.Status.State)
		return
	}

	// Reconcile owns a job's status until it records the launch, as in the
	// delete handler. A write before then races the launch write: KubeRay's
	// first status update for the RayJob CreateJob just created lands first, the
	// launch write conflicts and Launched=TRUE is lost, and the next pass calls
	// CreateJob again -- where a transient error fetching the remote RayCluster
	// records FAILED while the KubeRay RayJob runs on. Skipping also keeps a
	// leftover KubeRay RayJob of an earlier job with the same name off a job
	// that has not launched its own. The next KubeRay update, or the informer's
	// re-sync, delivers the status once the launch is recorded.
	if !isLaunched(&rayJob) {
		log.V(1).Info("skipping status update for job that is not launched yet")
		return
	}

	jobStatus, err := k8sengine.Mapper{}.MapLocalJobStatusToGlobal(local)
	if err != nil || jobStatus == nil || jobStatus.Ray == nil {
		log.Error(err, "could not map local ray job status")
		return
	}
	mapped := jobStatus.Ray

	// Cache re-sync gives Update events even when the job did not change.
	// Pre-check to avoid unnecessary CRD object updates.
	if rayJob.Status.State == mapped.State &&
		rayJob.Status.JobStatus == mapped.JobStatus &&
		rayJob.Status.JobDeploymentStatus == mapped.JobDeploymentStatus &&
		rayJob.Status.Message == mapped.Message {
		return
	}

	log.Info("ray job event",
		"kuberay_job_status", mapped.JobStatus,
		"kuberay_deployment_status", mapped.JobDeploymentStatus,
		"mapped_state", mapped.State)

	if err = retry.OnError(retry.DefaultBackoff, isRetriableWriteError, func() error {
		var current v2pb.RayJob
		if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
			return getErr
		}
		// Re-check on the fresh copy: a conflict means another writer got in
		// first and may have frozen, deleted or finished the job, or replaced it
		// with an unlaunched one of the same name.
		if utils.IsImmutable(&current) || current.GetDeletionTimestamp() != nil ||
			isTerminalRayJobState(current.Status.State) || !isLaunched(&current) {
			return nil
		}
		current.Status.State = mapped.State
		current.Status.JobStatus = mapped.JobStatus
		current.Status.JobDeploymentStatus = mapped.JobDeploymentStatus
		current.Status.Message = mapped.Message
		return r.Status().Update(ctx, &current)
	}); err != nil {
		log.Error(err, "could not update the global ray job status")
	}
}

// isLaunched reports whether Reconcile has recorded the job's launch.
func isLaunched(job *v2pb.RayJob) bool {
	launched := jobsutils.GetCondition(&job.Status.StatusConditions, constants.LaunchedCondition, job.Generation)
	return launched.Status == apipb.CONDITION_STATUS_TRUE
}

// rayJobDeleteEventHandler handles delete events for KubeRay RayJob resources.
func (r *Reconciler) rayJobDeleteEventHandler(obj interface{}) {
	// Recover the last-known RayJob from a tombstone if the informer missed the
	// final delete; the project namespace is only available from its labels.
	var local *rayv1.RayJob
	switch v := obj.(type) {
	case *rayv1.RayJob:
		local = v
	case cache.DeletedFinalStateUnknown:
		r.logger.Info("received tombstone delete event for ray job", "key", v.Key)
		j, ok := v.Obj.(*rayv1.RayJob)
		if !ok {
			r.logger.Error(fmt.Errorf("could not extract RayJob from tombstone, unexpected object type %T", v.Obj), "skipping delete event")
			return
		}
		local = j
	default:
		r.logger.Error(fmt.Errorf("unexpected object type %T in delete handler", v), "skipping delete event")
		return
	}
	log := r.logger.WithValues("ray_job", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the ray job")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayJob v2pb.RayJob
	if err = r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &rayJob); err != nil {
		if utils.IsNotFoundError(err) {
			// The global RayJob is gone: the ingester moved a terminal job to metadata
			// storage, or it was deleted outright. The KubeRay object outlives it until
			// owner GC removes it with its KubeRay RayCluster (KubeRay skips TTL cleanup
			// for jobs that target an existing cluster), so its trailing events are
			// expected and there is nothing to update.
			log.V(1).Info("global ray job no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global ray job")
		return
	}

	// A job that already reached a terminal state has its outcome recorded. KubeRay
	// removing the object afterwards -- owner GC when its KubeRay RayCluster is torn
	// down, an operator sweep -- is garbage collection, not a kill, so the terminal
	// record stands.
	// Terminal status and immutability are written by separate passes, so both are
	// checked to close the window between them.
	if utils.IsImmutable(&rayJob) || isTerminalRayJobState(rayJob.Status.State) {
		log.Info("skipping delete event for ray job that already reached a terminal state", "state", rayJob.Status.State)
		return
	}

	killing := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.KillingCondition, rayJob.Generation)

	if killing.Status != apipb.CONDITION_STATUS_TRUE {
		// An unlaunched job has no KubeRay RayJob of its own, so the deleted object
		// belongs to something else, e.g. an earlier job with the same name.
		launched := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.LaunchedCondition, rayJob.Generation)
		if launched.Status != apipb.CONDITION_STATUS_TRUE {
			log.V(1).Info("skipping delete event for ray job that was not launched")
			return
		}

		// Job was deleted externally without going through the controller.
		// wrote records whether the last attempt wrote the status: a retry after
		// a conflict may instead find the job already terminal.
		wrote := false
		if err = retry.OnError(retry.DefaultBackoff, isRetriableWriteError, func() error {
			wrote = false
			var current v2pb.RayJob
			if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
				return getErr
			}
			// Re-check on the fresh copy: another writer may have finished the job.
			if utils.IsImmutable(&current) || isTerminalRayJobState(current.Status.State) {
				return nil
			}
			markRayJobGone(&current, string(local.Status.JobStatus))
			wrote = true
			return r.Status().Update(ctx, &current)
		}); err != nil {
			log.Error(err, "failed to update status on external delete")
			return
		}
		if !wrote {
			// Another writer recorded the terminal state first. It stands, and the
			// reconcile that its write triggers freezes it.
			log.V(1).Info("skipping: ray job reached a terminal state concurrently")
			return
		}
		if err = r.markImmutableIfTerminal(ctx, &rayJob); err != nil {
			log.Error(err, "failed to mark ray job immutable after external delete")
			return
		}
		log.Info("job externally deleted, marked as failed")
		return
	}

	// Expected deletion — killing was in progress. Nothing sets KillingCondition
	// on a RayJob yet; this is reserved for a future RayJob kill path.
	wrote := false
	if err = retry.OnError(retry.DefaultBackoff, isRetriableWriteError, func() error {
		wrote = false
		var current v2pb.RayJob
		if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
			return getErr
		}
		// Re-check on the fresh copy: another writer may have finished the job.
		if utils.IsImmutable(&current) || isTerminalRayJobState(current.Status.State) {
			return nil
		}
		killingCond := jobsutils.GetCondition(&current.Status.StatusConditions, constants.KillingCondition, current.Generation)
		jobsutils.UpdateCondition(killingCond, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: current.Generation,
		})
		killed := jobsutils.GetCondition(&current.Status.StatusConditions, constants.KilledCondition, current.Generation)
		jobsutils.UpdateCondition(killed, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_TRUE,
			Generation: current.Generation,
		})
		current.Status.State = v2pb.RAY_JOB_STATE_KILLED
		wrote = true
		return r.Status().Update(ctx, &current)
	}); err != nil {
		log.Error(err, "failed to update status on expected delete")
		return
	}
	if !wrote {
		log.V(1).Info("skipping: ray job reached a terminal state concurrently")
		return
	}
	if err = r.markImmutableIfTerminal(ctx, &rayJob); err != nil {
		log.Error(err, "failed to mark ray job immutable after kill")
		return
	}
	log.Info("job killed successfully")
}

// markRayJobGone records that a launched job's KubeRay RayJob is gone from the
// compute cluster. The delete handler records it when it sees the delete, and
// the safety net when it finds the object missing, so both leave the same
// record. lastJobStatus is the job's last known KubeRay job status.
//
// The backing object is gone, so the job is terminal. Recording the state
// alongside the conditions is what lets markImmutableIfTerminal freeze the job:
// without it the CR keeps a non-terminal state forever, is never archived by
// the ingester, and re-reconciles on every requeue. FAILED rather than KILLED:
// the usual cause is the KubeRay RayCluster being deleted, which takes its
// RayJobs with it, and Uniflow retries a FAILED job but not a KILLED one.
func markRayJobGone(job *v2pb.RayJob, lastJobStatus string) {
	succeeded := jobsutils.GetCondition(&job.Status.StatusConditions, constants.SucceededCondition, job.Generation)
	jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_FALSE,
		Reason:     constants.ClusterKilled,
		Generation: job.Generation,
		Message:    fmt.Sprintf("ray job was deleted externally, status: %s", lastJobStatus),
	})
	killed := jobsutils.GetCondition(&job.Status.StatusConditions, constants.KilledCondition, job.Generation)
	jobsutils.UpdateCondition(killed, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Generation: job.Generation,
	})
	job.Status.State = v2pb.RAY_JOB_STATE_FAILED
	job.Status.JobStatus = _jobStatusUnknown
	job.Status.Message = "ray job was deleted from the compute cluster"
}
