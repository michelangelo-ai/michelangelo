package job

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/watch"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	requeueAfter = time.Second * 10
	apiVersion   = "ray.io/v1"
)

// Reconciler reconciles a Ray Job object
type Reconciler struct {
	client.Client
	logger           logr.Logger
	federatedClient  jobsclient.FederatedClient
	clusterCache     jobscluster.RegisteredClustersCache
	env              env.Context
	federatedWatcher watch.FederatedWatcher
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
	metricsScope tally.Scope,
) *Reconciler {
	return &Reconciler{
		logger:          logger,
		Client:          client,
		federatedClient: federatedClient,
		clusterCache:    clusterCache,
		env:             env,
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

	if !reflect.DeepEqual(originalRayJob, rayJob) {
		// update the resource in ETCD
		if isTerminalRayJobState(rayJob.Status.State) {
			utils.MarkImmutable(&rayJob)
		}
		err := r.Status().Update(ctx, &rayJob)
		if err != nil {
			logger.Error(err, "failed to update status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	logger.Info("reconcile finished, re-queue after ", "requeueAfter", res.RequeueAfter)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger().WithName("rayjob")

	r.federatedWatcher = r.getFederatedWatcher()

	go func() {
		<-mgr.Elected()
		r.federatedWatcher.Start(context.TODO())
	}()

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

	if !r.ensureClusterReady(ctx, logger, rayJob, rayCluster, res) {
		return // Cluster not ready, will requeue
	}

	launched := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.LaunchedCondition, rayJob.Generation)
	if launched.Status != apipb.CONDITION_STATUS_TRUE {
		r.createRayJobIfNotLaunched(ctx, logger, rayJob, rayCluster, res)
	} else {
		r.updateJobStatusIfLaunched(ctx, logger, rayJob, rayCluster, res)
	}
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

// updateJobStatusIfLaunched handles job status after launch.
// Job status is now updated by the federated watcher event handlers
// rather than polling via GetJobStatus.
func (r *Reconciler) updateJobStatusIfLaunched(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob, rayCluster *v2pb.RayCluster, res *ctrl.Result) {
	if !isTerminalRayJobState(rayJob.Status.State) {
		res.RequeueAfter = requeueAfter
	}
}

func isTerminalRayJobState(state v2pb.RayJobState) bool {
	switch state {
	case v2pb.RAY_JOB_STATE_FAILED, v2pb.RAY_JOB_STATE_SUCCEEDED, v2pb.RAY_JOB_STATE_KILLED:
		return true
	default:
		return false
	}
}
