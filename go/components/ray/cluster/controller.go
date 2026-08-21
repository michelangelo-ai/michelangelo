package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// Defines the delay before retrying the reconciliation process
	requeueAfter            = time.Second * 10
	_updateStatusCtxTimeout = time.Second * 10
)

// Condition constants for tracking RayCluster lifecycle
const (
	EnqueuedCondition  = constants.EnqueuedCondition
	ScheduledCondition = constants.ScheduledCondition
	LaunchedCondition  = constants.LaunchedCondition
	KillingCondition   = constants.KillingCondition
	KilledCondition    = constants.KilledCondition
	SucceededCondition = constants.SucceededCondition
)

// Reconciler handles the lifecycle of Ray Cluster objects in the Kubernetes cluster
type Reconciler struct {
	api.Handler // API client for managing API objects

	logger            logr.Logger                         // Logger for the controller
	apiHandlerFactory apiHandler.Factory                  // Factory for creating API handlers
	env               env.Context                         // Environment context for configuration
	schedulerQueue    scheduler.JobQueue                  // Queue for enqueuing jobs to scheduler
	federatedClient   jobsclient.FederatedClient          // Client for creating clusters on remote K8s
	clusterCache      jobscluster.RegisteredClustersCache // Cache for looking up assigned clusters
	metricsScope      tally.Scope                         // Metrics scope for telemetry
}

// NewReconciler constructs a Reconciler with required dependencies.
//
// This provides a stable construction API for downstream users so they do not
// need to rely on reflection to set unexported fields.
func NewReconciler(
	logger logr.Logger,
	apiHandlerFactory apiHandler.Factory,
	env env.Context,
	schedulerQueue scheduler.JobQueue,
	federatedClient jobsclient.FederatedClient,
	clusterCache jobscluster.RegisteredClustersCache,
	metricsScope tally.Scope,
) *Reconciler {
	return &Reconciler{
		logger:            logger,
		apiHandlerFactory: apiHandlerFactory,
		env:               env,
		schedulerQueue:    schedulerQueue,
		federatedClient:   federatedClient,
		clusterCache:      clusterCache,
		metricsScope:      metricsScope,
	}
}

// Reconcile ensures the desired state of the Ray Cluster matches the actual state in the cluster.
// It implements the Kubernetes reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Initialize logger from the context for scoped logging
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ray cluster", "namespacedName", req.NamespacedName)

	// Initialize the result object to define next actions
	res := ctrl.Result{}

	// Retrieve the RayCluster custom resource using the request's namespace and name
	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, &rayCluster); err != nil {
		// If the resource is not found, assume it has been deleted
		if utils.IsNotFoundError(err) {
			logger.Info("RayCluster not found, skipping reconciliation")
			return ctrl.Result{}, nil
		}
		// Requeue for errors other than not found
		logger.Error(err, "failed to get ray cluster")
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	// Check for termination
	if r.shouldTerminateCluster(&rayCluster) {
		logger.Info("processing cluster termination")
		err := r.processClusterTermination(ctx, &rayCluster, logger)
		if err != nil {
			logger.Error(err, "cluster termination could not be processed")
			return ctrl.Result{RequeueAfter: requeueAfter}, err
		}
		logger.Info("processed cluster termination")
		return ctrl.Result{}, nil
	}

	// Enqueue if not scheduled
	err := r.enqueueIfRequired(ctx, &rayCluster, logger)
	if err != nil {
		logger.Error(err, "failed to enqueue cluster")
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	// Wait for scheduling
	assignedCluster := r.getClusterIfScheduled(&rayCluster)
	if assignedCluster == nil {
		logger.Info("cluster not yet scheduled, requeue for later")
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Create RayCluster if not launched
	launched := jobsutils.GetCondition(&rayCluster.Status.StatusConditions, LaunchedCondition, rayCluster.Generation)
	if launched.Status != apipb.CONDITION_STATUS_TRUE {
		logger.Info("creating ray cluster via federated client", "assignedCluster", assignedCluster.Name)
		err = r.federatedClient.CreateJobCluster(ctx, &rayCluster, assignedCluster)
		if err != nil {
			if apiErrors.IsAlreadyExists(err) {
				logger.Info("cluster already exists, treating as launched")
				// Continue to update launched condition
			} else {
				logger.Error(err, "failed to create ray cluster via federated client")
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED

				if err = jobsutils.UpdateStatusWithRetries(
					ctx, r, &rayCluster,
					func(obj client.Object) {
						cluster := obj.(*v2pb.RayCluster)
						cluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED

						// Set failed condition to trigger termination
						succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
						jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
							Status:     apipb.CONDITION_STATUS_FALSE,
							Generation: cluster.Generation,
							Reason:     "ClusterCreationFailed",
							Message:    err.Error(),
						})
					},
					&metav1.UpdateOptions{},
				); err != nil {
					logger.Error(err, "failed to update status after creation failure")
				}

				return ctrl.Result{RequeueAfter: requeueAfter}, err
			}
		}

		// Update LaunchedCondition to TRUE and set state to PROVISIONING
		if err = jobsutils.UpdateStatusWithRetries(
			ctx, r, &rayCluster,
			func(obj client.Object) {
				cluster := obj.(*v2pb.RayCluster)
				cluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING

				launchedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, LaunchedCondition, cluster.Generation)
				jobsutils.UpdateCondition(launchedCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Generation: cluster.Generation,
					Reason:     "ClusterCreated",
				})
			},
			&metav1.UpdateOptions{},
		); err != nil {
			logger.Error(err, "failed to update launched condition")
			return ctrl.Result{RequeueAfter: requeueAfter}, err
		}
		logger.Info("cluster creation initiated")
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Status is now synced by the federated watcher event handlers.
	// No polling required once the cluster is launched.

	logger.Info("Reconcile finished")
	return res, nil
}

// Register adds the Reconciler to the controller manager
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger().WithName("raycluster")
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler

	federatedWatcher := r.getFederatedWatcher()
	go func() {
		<-mgr.Elected()
		federatedWatcher.Start(context.TODO())
	}()

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}). // Watch for changes in RayCluster custom resources
		Complete(r)
}

// shouldTerminateCluster checks if the cluster should be terminated
func (r *Reconciler) shouldTerminateCluster(cluster *v2pb.RayCluster) bool {
	shouldTerminate, _ := jobsutils.IsTerminationInfoSet(cluster)
	if shouldTerminate {
		return true
	}

	// If the cluster has been marked as failed/succeeded, then terminate
	success := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
	return success.Status != apipb.CONDITION_STATUS_UNKNOWN
}

// processClusterTermination handles the full termination flow for a cluster
func (r *Reconciler) processClusterTermination(
	ctx context.Context,
	cluster *v2pb.RayCluster,
	log logr.Logger,
) error {
	// Step 1: Set succeeded condition if required
	err := r.setClusterSuccessConditionIfRequired(ctx, cluster, log)
	if err != nil {
		return fmt.Errorf("could not set succeeded condition: %w", err)
	}

	// Step 2: Set killing condition if required
	err = r.setClusterKillIfRequired(ctx, cluster, log)
	if err != nil {
		return fmt.Errorf("could not set killing condition: %w", err)
	}

	// Step 3: Cleanup cluster resources
	err = r.cleanupCluster(ctx, cluster, log)
	if err != nil {
		return fmt.Errorf("could not perform cleanup: %w", err)
	}

	return nil
}

// setClusterSuccessConditionIfRequired sets the succeeded condition based on termination type
func (r *Reconciler) setClusterSuccessConditionIfRequired(
	ctx context.Context,
	cluster *v2pb.RayCluster,
	log logr.Logger,
) error {
	success := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
	if success.Status != apipb.CONDITION_STATUS_UNKNOWN {
		return nil
	}

	var status apipb.ConditionStatus
	switch cluster.Spec.Termination.Type {
	case v2pb.TERMINATION_TYPE_SUCCEEDED:
		status = apipb.CONDITION_STATUS_TRUE
	case v2pb.TERMINATION_TYPE_FAILED:
		status = apipb.CONDITION_STATUS_FALSE
	default:
		return fmt.Errorf("invalid termination type %s", cluster.Spec.Termination.Type)
	}

	if err := jobsutils.UpdateStatusWithRetries(ctx, r, cluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
			jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
				Status:     status,
				Generation: cluster.Generation,
				Reason:     cluster.Spec.Termination.Reason,
			})
		}, &metav1.UpdateOptions{
			FieldManager: "setClusterSuccessConditionIfRequired",
		}); err != nil {
		return err
	}

	log.Info("cluster succeeded condition set", "status", status)
	return nil
}

// setClusterKillIfRequired sets the killing condition if the cluster can be killed
func (r *Reconciler) setClusterKillIfRequired(
	ctx context.Context,
	cluster *v2pb.RayCluster,
	log logr.Logger,
) error {
	killing := jobsutils.GetCondition(&cluster.Status.StatusConditions, KillingCondition, cluster.Generation)
	if killing.Status != apipb.CONDITION_STATUS_UNKNOWN {
		return nil
	}

	killed := jobsutils.GetCondition(&cluster.Status.StatusConditions, KilledCondition, cluster.Generation)
	if killed.Status == apipb.CONDITION_STATUS_TRUE {
		return nil
	}

	if err := jobsutils.UpdateStatusWithRetries(ctx, r, cluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			killing := jobsutils.GetCondition(&cluster.Status.StatusConditions, KillingCondition, cluster.Generation)
			jobsutils.UpdateCondition(killing, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: cluster.Generation,
			})
		}, &metav1.UpdateOptions{
			FieldManager: "setClusterKillIfRequired",
		}); err != nil {
		return err
	}

	log.Info("cluster killing condition set to true")
	return nil
}

// cleanupCluster performs cleanup of cluster resources
func (r *Reconciler) cleanupCluster(
	ctx context.Context,
	cluster *v2pb.RayCluster,
	log logr.Logger,
) error {
	killing := jobsutils.GetCondition(&cluster.Status.StatusConditions, KillingCondition, cluster.Generation)
	if killing.Status != apipb.CONDITION_STATUS_TRUE {
		return nil
	}

	assignedCluster := r.getClusterIfScheduled(cluster)

	// Cluster not scheduled yet
	if assignedCluster == nil {
		log.Info("cluster has not been scheduled yet, setting killed state")
		if err := jobsutils.UpdateStatusWithRetries(ctx, r, cluster,
			func(obj client.Object) {
				currentCluster := obj.(*v2pb.RayCluster)
				killedCond := jobsutils.GetCondition(&currentCluster.Status.StatusConditions, KilledCondition, currentCluster.Generation)
				jobsutils.UpdateCondition(killedCond, jobsutils.ConditionUpdateParams{
					Status: apipb.CONDITION_STATUS_TRUE,
					Reason: constants.KilledMessageJobNotLaunched,
				})
				killingCond := jobsutils.GetCondition(&currentCluster.Status.StatusConditions, KillingCondition, currentCluster.Generation)
				jobsutils.UpdateCondition(killingCond, jobsutils.ConditionUpdateParams{
					Status: apipb.CONDITION_STATUS_FALSE,
				})
				currentCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
			}, &metav1.UpdateOptions{
				FieldManager: "cleanupClusterNotScheduled",
			}); err != nil {
			log.Error(err, "could not update the cluster status")
			return err
		}
		return nil
	}

	// Cluster scheduled but not launched yet
	launched := jobsutils.GetCondition(&cluster.Status.StatusConditions, LaunchedCondition, cluster.Generation)
	if launched.Status != apipb.CONDITION_STATUS_TRUE {
		log.Info("cluster has not been launched yet, setting killed state")
		if err := jobsutils.UpdateStatusWithRetries(ctx, r, cluster,
			func(obj client.Object) {
				currentCluster := obj.(*v2pb.RayCluster)
				killedCond := jobsutils.GetCondition(&currentCluster.Status.StatusConditions, KilledCondition, currentCluster.Generation)
				jobsutils.UpdateCondition(killedCond, jobsutils.ConditionUpdateParams{
					Status: apipb.CONDITION_STATUS_TRUE,
					Reason: constants.KilledMessageJobNotLaunched,
				})
				killingCond := jobsutils.GetCondition(&currentCluster.Status.StatusConditions, KillingCondition, currentCluster.Generation)
				jobsutils.UpdateCondition(killingCond, jobsutils.ConditionUpdateParams{
					Status: apipb.CONDITION_STATUS_FALSE,
				})
				currentCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
			}, &metav1.UpdateOptions{
				FieldManager: "cleanupClusterNotLaunched",
			}); err != nil {
			log.Error(err, "could not update the cluster status")
			return err
		}
		return nil
	}

	// Cluster launched, delete via federated client
	err := r.federatedClient.DeleteJobCluster(ctx, cluster, assignedCluster)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			log.Info("cluster already deleted")
			// Still update status to terminated
		} else {
			return fmt.Errorf("could not delete cluster: %w", err)
		}
	}

	// Update status to terminated.
	// The federated watcher will handle the killing->killed transition
	// when the RayCluster is actually deleted from the compute cluster.
	if err := jobsutils.UpdateStatusWithRetries(ctx, r, cluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			killingCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, KillingCondition, cluster.Generation)
			jobsutils.UpdateCondition(killingCond, jobsutils.ConditionUpdateParams{
				Status: apipb.CONDITION_STATUS_FALSE,
			})
			killedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, KilledCondition, cluster.Generation)
			jobsutils.UpdateCondition(killedCond, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: cluster.Generation,
			})
			cluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
		}, &metav1.UpdateOptions{
			FieldManager: "cleanupClusterDeleted",
		}); err != nil {
		log.Error(err, "could not update the cluster status")
		return err
	}

	log.Info("cluster cleaned up successfully")
	return nil
}

// enqueueIfRequired enqueues the cluster to the scheduler queue if not already enqueued
func (r *Reconciler) enqueueIfRequired(
	ctx context.Context,
	cluster *v2pb.RayCluster,
	log logr.Logger,
) error {
	enqueued := jobsutils.GetCondition(&cluster.Status.StatusConditions, EnqueuedCondition, cluster.Generation)
	enqueuedStatus := enqueued.Status

	ctx, cancel := context.WithTimeout(ctx, _updateStatusCtxTimeout)
	defer cancel()

	// First time encountering the cluster, enqueue it
	if enqueuedStatus != apipb.CONDITION_STATUS_TRUE {
		if err := jobsutils.UpdateStatusWithRetries(
			ctx, r, cluster,
			func(obj client.Object) {
				currentCluster := obj.(*v2pb.RayCluster)
				enqueued := jobsutils.GetCondition(&currentCluster.Status.StatusConditions, EnqueuedCondition, currentCluster.Generation)
				jobsutils.UpdateCondition(enqueued, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Reason:     constants.AddedToSchedulerQueue,
					Generation: currentCluster.Generation,
				})
			},
			&metav1.UpdateOptions{},
		); err != nil {
			return fmt.Errorf("status update err: %v", err)
		}

		// Enqueue after status update to avoid conflicts
		if err := r.schedulerQueue.Enqueue(ctx, matypes.NewSchedulableJob(matypes.SchedulableJobParams{
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
			Generation: cluster.Generation,
			JobType:    matypes.RayCluster,
		})); err != nil {
			if errors.Is(err, matypes.ErrJobAlreadyExists) {
				// do not report error
				log.V(1).Info("job already exists in the scheduler queue")
				return nil
			}
			return err
		}
		log.Info("enqueued cluster")
		return nil
	}

	// In restart cases, scheduled clusters need to be placed back on the queue
	scheduled := jobsutils.GetCondition(&cluster.Status.StatusConditions, ScheduledCondition, cluster.Generation)
	scheduledStatus := scheduled.Status
	if scheduledStatus != apipb.CONDITION_STATUS_TRUE {
		if err := r.schedulerQueue.Enqueue(ctx, matypes.NewSchedulableJob(matypes.SchedulableJobParams{
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
			Generation: cluster.Generation,
			JobType:    matypes.RayCluster,
		})); err != nil {
			if errors.Is(err, matypes.ErrJobAlreadyExists) {
				log.V(1).Info("cluster already exists in the scheduler queue")
				return nil
			}
			return err
		}

		log.Info("enqueued cluster because not yet scheduled", "scheduled_condition_status", scheduledStatus)
	}
	return nil
}

// getClusterIfScheduled returns the assigned cluster if the RayCluster has been scheduled
func (r *Reconciler) getClusterIfScheduled(cluster *v2pb.RayCluster) *v2pb.Cluster {
	isScheduled := jobsutils.IsJobScheduled(cluster.Status.StatusConditions, cluster.Generation)
	if !isScheduled {
		return nil
	}

	if cluster.Status.Assignment == nil || cluster.Status.Assignment.Cluster == "" {
		return nil
	}

	assignedCluster := r.clusterCache.GetCluster(cluster.Status.Assignment.Cluster)
	return assignedCluster
}
