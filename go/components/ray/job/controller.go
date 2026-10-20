package job

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/controllerutil"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/watch"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	requeueAfter = time.Second * 10
	apiVersion   = "ray.io/v1"

	// _safetyNetPeriod is how often a launched job's KubeRay RayJob is polled as
	// a fallback for events the watcher misses.
	_safetyNetPeriod = 5 * time.Minute

	// _remoteCallTimeout bounds each safety-net call to a compute cluster. The
	// federated client sets no request timeout and the controller runs a single
	// worker, so an unresponsive compute cluster would otherwise stall the
	// reconciliation of every RayJob.
	_remoteCallTimeout = 30 * time.Second
)

// Reconciler reconciles a Ray Job object
type Reconciler struct {
	client.Client
	logger           logr.Logger
	federatedClient  jobsclient.FederatedClient
	clusterCache     jobscluster.RegisteredClustersCache
	env              env.Context
	federatedWatcher watch.FederatedWatcher
	mapper           matypes.Mapper
	metricsScope     tally.Scope
}

// NewReconciler constructs a Reconciler with required dependencies.
//
// This provides a stable construction API for downstream users so they do not
// need to rely on reflection to set unexported fields.
func NewReconciler(
	logger logr.Logger,
	client client.Client,
	env env.Context,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobscluster.RegisteredClustersCache,
	mapper matypes.Mapper,
	metricsScope tally.Scope,
) *Reconciler {
	if metricsScope == nil {
		metricsScope = tally.NoopScope
	}
	return &Reconciler{
		logger:          logger,
		Client:          client,
		federatedClient: federatedClient,
		clusterCache:    clusterCache,
		env:             env,
		mapper:          mapper,
		metricsScope:    metricsScope,
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("namespacedName", req.NamespacedName)
	logger.Info("Reconciling ray job")
	res := ctrl.Result{}

	// retrieve the ray job
	var rayJob v2pb.RayJob
	if err := r.Get(ctx, req.NamespacedName, &rayJob); err != nil {
		// Resource not found (resource deleted)
		if utils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}

	// An immutable RayJob has reached a terminal state and is being moved to metadata
	// storage by the ingester (which will delete it from ETCD). Skip reconciliation so we
	// neither waste queue capacity nor re-trigger ourselves by rewriting a terminal object.
	if utils.IsImmutable(&rayJob) {
		logger.Info("RayJob is immutable, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// A terminal state is final; all that is left is to freeze it. The watcher
	// records the terminal state and this pass freezes it, so return before the
	// cluster checks below: by now Uniflow may be tearing the cluster down, and
	// re-deriving the state from it would overwrite the recorded outcome.
	if isTerminalRayJobState(rayJob.Status.State) {
		if err := r.markImmutableIfTerminal(ctx, &rayJob); err != nil {
			logger.Error(err, "failed to persist immutable annotation")
			return ctrl.Result{RequeueAfter: requeueAfter}, err
		}
		return ctrl.Result{}, nil
	}

	// original copy of ray job to determine if we need to update the status
	originalRayJob := rayJob.DeepCopy()
	// Initialize status conditions, as they will be nil for new jobs
	if rayJob.GetStatus().StatusConditions == nil {
		rayJob.Status.StatusConditions = make([]*apipb.Condition, 0)
	}

	// Handle missing cluster spec
	if rayJob.Spec.Cluster == nil {
		rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
		rayJob.Status.Message = "cluster is not set"
	} else {
		r.reconcileRayJobWithCluster(ctx, logger, &rayJob, &res)
	}

	if !reflect.DeepEqual(originalRayJob.Status, rayJob.Status) {
		// update the resource in ETCD
		err := r.Status().Update(ctx, &rayJob)
		if apiErrors.IsConflict(err) {
			// Another writer updated the job after this pass read it. That is
			// routine, and its write triggers a fresh pass on the current object.
			logger.V(1).Info("status update conflicted with a concurrent write, requeueing")
			return ctrl.Result{Requeue: true}, nil
		}
		if err != nil {
			logger.Error(err, "failed to update status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	// Mark jobs that turned terminal in this pass immutable so the ingester can
	// move them to metadata storage and delete the CR. A dropped write is retried
	// by the terminal check at the top of the next pass, so it cannot strand the
	// job.
	if isTerminalRayJobState(rayJob.Status.State) && !utils.IsImmutable(&rayJob) {
		if err := r.markImmutableIfTerminal(ctx, &rayJob); err != nil {
			// The status write above already committed; requeue to retry the
			// annotation rather than dropping it best-effort.
			logger.Error(err, "failed to persist immutable annotation")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	logger.Info("reconcile finished, re-queue after ", "requeueAfter", res.RequeueAfter)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	// The watcher maps every KubeRay event through the mapper. Without one each
	// event would panic in an informer goroutine, so refuse to start instead.
	if r.mapper == nil {
		return errors.New("ray job controller: a mapper is required")
	}
	r.logger = mgr.GetLogger().WithName("rayjob")

	r.federatedWatcher = r.getFederatedWatcher()

	// Leader-only runnables start once the manager's caches have synced and get
	// its context, so the watcher stops with the manager. Start blocks until then.
	if err := mgr.Add(controllerutil.LeaderOnlyRunnable(func(ctx context.Context) error {
		r.federatedWatcher.Start(ctx)
		return nil
	})); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayJob{}).
		Complete(r)
}

// reconcileRayJobWithCluster handles the reconciliation logic when cluster spec is provided.
func (r *Reconciler) reconcileRayJobWithCluster(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, res *ctrl.Result) {
	rayCluster := r.fetchRayCluster(ctx, logger, rayJob, res)
	if rayCluster == nil {
		return // Error already handled in fetchRayCluster
	}

	// Cluster readiness gates only the launch. After launch the watcher owns
	// State and Message, so a cluster leaving READY must not rewrite them.
	launched := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.LaunchedCondition, rayJob.Generation)
	if launched.Status == apipb.CONDITION_STATUS_TRUE {
		r.updateJobStatusIfLaunched(ctx, logger, rayJob, rayCluster, res)
		return
	}

	if !r.ensureClusterReady(ctx, logger, rayJob, rayCluster, res) {
		return // Cluster not ready, will requeue
	}
	r.createRayJobIfNotLaunched(ctx, logger, rayJob, rayCluster, res)
}

// fetchRayCluster retrieves the RayCluster resource referenced by the RayJob.
// Returns the cluster if found, nil otherwise (error handling is done internally).
func (r *Reconciler) fetchRayCluster(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, res *ctrl.Result) *v2pb.RayCluster {
	rayCluster := &v2pb.RayCluster{}
	clusterRef := rayJob.GetSpec().Cluster

	err := r.Get(ctx, types.NamespacedName{
		Namespace: clusterRef.GetNamespace(),
		Name:      clusterRef.GetName(),
	}, rayCluster)
	if err != nil {
		if utils.IsNotFoundError(err) {
			rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
			rayJob.Status.Message = fmt.Sprintf("failed to find cluster %s/%s", clusterRef.GetNamespace(), clusterRef.GetName())
			return nil
		}
		logger.Error(err, "error to get cluster, requeue")
		res.RequeueAfter = requeueAfter
		return nil
	}

	return rayCluster
}

// ensureClusterReady checks if the RayCluster is in ready state.
// Returns true if ready, false otherwise (will requeue).
func (r *Reconciler) ensureClusterReady(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, rayCluster *v2pb.RayCluster, res *ctrl.Result) bool {
	if rayCluster.Status.State != v2pb.RAY_CLUSTER_STATE_READY {
		logger.Info("cluster is not ready, waiting")
		// Reflect waiting state while the cluster becomes ready
		rayJob.Status.State = v2pb.RAY_JOB_STATE_INITIALIZING
		rayJob.Status.Message = fmt.Sprintf("cluster %s/%s is not ready", rayCluster.Namespace, rayCluster.Name)
		res.RequeueAfter = requeueAfter
		return false
	}
	return true
}

// getAssignedCluster retrieves the assigned physical cluster from the RayCluster status.
// Returns the cluster if found, nil otherwise.
func (r *Reconciler) getAssignedCluster(logger logr.Logger, rayCluster *v2pb.RayCluster) *v2pb.Cluster {
	assignment := rayCluster.GetStatus().Assignment
	if assignment == nil || assignment.GetCluster() == "" {
		return nil
	}

	clusterName := assignment.GetCluster()
	assignedCluster := r.clusterCache.GetCluster(clusterName)
	if assignedCluster == nil {
		logger.Error(fmt.Errorf("cluster not found"), "assigned cluster not in cache", "cluster", clusterName)
		return nil
	}

	return assignedCluster
}

// createRayJobIfNotLaunched creates the Ray job if it hasn't been launched yet.
func (r *Reconciler) createRayJobIfNotLaunched(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, rayCluster *v2pb.RayCluster, res *ctrl.Result) {
	assignedCluster := r.getAssignedCluster(logger, rayCluster)
	if assignedCluster == nil {
		logger.Info("RayCluster not yet assigned to a physical cluster")
		rayJob.Status.Message = "waiting for RayCluster assignment"
		res.RequeueAfter = requeueAfter
		return
	}

	err := r.federatedClient.CreateJob(ctx, rayJob, rayCluster, assignedCluster)
	if err != nil && !apiErrors.IsAlreadyExists(err) {
		logger.Error(err, "failed to create ray job via federated client")
		rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
		rayJob.Status.Message = fmt.Sprintf("failed to create ray job: %v", err)
		res.RequeueAfter = requeueAfter
		return
	}

	// Mark as launched
	rayJob.Status.State = v2pb.RAY_JOB_STATE_INITIALIZING
	launchedCond := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.LaunchedCondition, rayJob.Generation)
	jobsutils.UpdateCondition(launchedCond, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Generation: rayJob.Generation,
		Reason:     "Launched",
	})
	res.RequeueAfter = requeueAfter
}

// updateJobStatusIfLaunched is the level-triggered safety net behind the
// federated watcher, which owns a launched job's status. Every _safetyNetPeriod
// it polls the KubeRay RayJob and applies the result only if it is terminal, so
// the job still finishes when the watcher misses the event: a delete while the
// controller was down or the compute cluster was out of the ready set, a dropped
// status write, or a KubeRay RayJob created before the watcher's labels existed.
// A KubeRay RayJob missing from the compute cluster is recorded as the delete
// handler records a delete. Non-terminal results are left to the watcher.
func (r *Reconciler) updateJobStatusIfLaunched(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, rayCluster *v2pb.RayCluster, res *ctrl.Result) {
	assignedCluster := r.getAssignedCluster(logger, rayCluster)
	if assignedCluster == nil {
		logger.Info("assigned cluster not found, skipping ray job status check")
		res.RequeueAfter = _safetyNetPeriod
		return
	}

	remoteCtx, cancel := context.WithTimeout(ctx, _remoteCallTimeout)
	defer cancel()
	jobStatus, err := r.federatedClient.GetJobStatus(remoteCtx, rayJob, assignedCluster)
	if err != nil {
		// Running out of time is like any other remote error: try again next
		// period.
		logger.Error(err, "error to get ray job status")
		res.RequeueAfter = _safetyNetPeriod
		return
	}
	if jobStatus == nil || jobStatus.Ray == nil || !isTerminalRayJobState(jobStatus.Ray.State) {
		res.RequeueAfter = _safetyNetPeriod
		return
	}
	if isRayJobMissing(jobStatus.Ray) {
		// The delete the watcher never saw: record it as the delete handler would.
		markRayJobGone(rayJob, rayJob.Status.JobStatus)
		return
	}

	rayJob.Status.State = jobStatus.Ray.State
	rayJob.Status.JobStatus = jobStatus.Ray.JobStatus
	rayJob.Status.JobDeploymentStatus = jobStatus.Ray.JobDeploymentStatus
	rayJob.Status.Message = jobStatus.Ray.Message
}

// isRayJobMissing reports whether a GetJobStatus result stands for a KubeRay
// RayJob missing from the compute cluster. GetJobStatus returns the compute
// cluster API server's NotFound as a FAILED status with JobStatus
// jobsclient.JobStatusNotFound rather than an error; any other failure, a
// proxy's 404 included, is an error and leaves the job as it is.
func isRayJobMissing(status *v2pb.RayJobStatus) bool {
	return status.State == v2pb.RAY_JOB_STATE_FAILED && status.JobStatus == jobsclient.JobStatusNotFound
}

// markImmutableIfTerminal persists the michelangelo/Immutable annotation on a
// terminal RayJob so the ingester can move it to metadata storage and delete the
// CR; afterwards Get returns NotFound and reconciliation stops. It is a no-op if
// the job is already immutable or is no longer terminal.
//
// The marker is an annotation (metadata), which Status().Update cannot persist,
// so this uses a plain Update. It re-fetches the object first -- the caller just
// wrote the status subresource -- and retries on conflict, keeping the
// resourceVersion current against that write and any concurrent writer.
func (r *Reconciler) markImmutableIfTerminal(ctx context.Context, rayJob *v2pb.RayJob) error {
	// A conflict here means a concurrent writer -- the reconcile loop and the
	// watcher both reach this path -- won the race. The closure re-fetches, so a
	// retry simply observes their write and no-ops; without this the caller would
	// surface a spurious error for an outcome that is already correct.
	isRetriable := func(err error) bool {
		return apiErrors.IsConflict(err) || jobsutils.IsRetriableError(err)
	}
	if err := retry.OnError(retry.DefaultRetry, isRetriable, func() error {
		latest := &v2pb.RayJob{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: rayJob.Namespace, Name: rayJob.Name}, latest); err != nil {
			return err
		}
		if utils.IsImmutable(latest) || !isTerminalRayJobState(latest.Status.State) {
			return nil
		}
		utils.MarkImmutable(latest)
		return r.Update(ctx, latest)
	}); err != nil {
		return fmt.Errorf("failed to mark ray job immutable: %w", err)
	}
	return nil
}

func isTerminalRayJobState(state v2pb.RayJobState) bool {
	switch state {
	case v2pb.RAY_JOB_STATE_FAILED, v2pb.RAY_JOB_STATE_SUCCEEDED, v2pb.RAY_JOB_STATE_KILLED:
		return true
	default:
		return false
	}
}
