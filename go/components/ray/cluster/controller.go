// Package cluster implements a Kubernetes controller for managing RayCluster resources.
//
// This package provides a reconciler that manages the complete lifecycle of Ray clusters,
// from initial creation through scheduling, provisioning, monitoring, and termination.
// It integrates with the job scheduler and federated cluster clients to deploy Ray clusters
// across multiple Kubernetes environments.
//
// Lifecycle States:
//
// RayCluster resources progress through the following states:
//   - INVALID → PROVISIONING: Cluster scheduled and creation initiated
//   - PROVISIONING → READY: Cluster fully provisioned and operational
//   - READY → TERMINATED: Cluster terminated after completion or failure
//   - Any State → FAILED/UNHEALTHY: Error conditions trigger termination
//
// Condition-Based Flow:
//
// The controller uses status conditions to track progress:
//  1. Enqueued: Added to scheduler queue
//  2. Scheduled: Assigned to a physical Kubernetes cluster
//  3. Launched: Cluster created via federated client
//  4. Succeeded/Failed: Final outcome (triggers termination)
//  5. Killing → Killed: Cleanup and resource deletion
//
// Integration:
//
//   - Job Scheduler: Queues clusters for resource allocation
//   - Federated Client: Creates/deletes clusters on remote Kubernetes clusters
//   - KubeRay: Underlying operator that manages Ray cluster pods
//
// TODO(#605): Implement federated watcher to eliminate polling for status updates
package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	jobscluster "github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	// requeueAfter defines the delay before retrying reconciliation after transient errors.
	requeueAfter = time.Second * 10
	// _updateStatusCtxTimeout specifies the timeout for status update operations.
	_updateStatusCtxTimeout = time.Second * 10
)

// Condition constants for tracking RayCluster lifecycle stages.
//
// These conditions are used to manage the cluster's progression through its lifecycle,
// from initial enqueueing through final termination. Conditions are generation-aware
// to handle resource updates correctly.
const (
	EnqueuedCondition  = constants.EnqueuedCondition  // Cluster added to scheduler queue
	ScheduledCondition = constants.ScheduledCondition // Cluster assigned to physical cluster
	LaunchedCondition  = constants.LaunchedCondition  // Cluster created via federated client
	KillingCondition   = constants.KillingCondition   // Cluster deletion in progress
	KilledCondition    = constants.KilledCondition    // Cluster successfully deleted
	SucceededCondition = constants.SucceededCondition // Cluster completed successfully or failed
)

// Reconciler manages the lifecycle of RayCluster custom resources.
//
// The reconciler implements a state machine that handles cluster scheduling, provisioning,
// monitoring, and termination. It coordinates between the job scheduler for resource
// allocation and the federated client for cluster creation on remote Kubernetes clusters.
//
// Key responsibilities:
//   - Enqueuing clusters to the job scheduler
//   - Monitoring cluster scheduling status
//   - Creating clusters via federated client when scheduled
//   - Polling cluster status from remote clusters (TODO: replace with watchers)
//   - Handling termination requests and cleanup
type Reconciler struct {
	api.Handler // API client for Kubernetes operations

	env             env.Context                         // Environment configuration context
	schedulerQueue  scheduler.JobQueue                  // Job scheduler queue for resource allocation
	federatedClient jobsclient.FederatedClient          // Client for remote cluster operations
	clusterCache    jobscluster.RegisteredClustersCache // Cache of available physical clusters
}

// Reconcile implements the Kubernetes reconciliation loop for RayCluster resources.
//
// This method is invoked whenever a RayCluster resource is created, updated, or periodically
// requeued. It processes the cluster through its lifecycle stages:
//
//  1. Check for termination conditions (user-requested or failure)
//  2. Enqueue cluster to scheduler if not already enqueued
//  3. Wait for scheduler to assign a physical cluster
//  4. Create cluster on assigned cluster via federated client
//  5. Monitor cluster status and update local resource
//  6. Handle termination and cleanup when requested
//
// Returns ctrl.Result with RequeueAfter set for ongoing monitoring, or an error
// if reconciliation should be retried by the framework.
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

	// Create a copy of the original RayCluster for comparison
	originalRayCluster := rayCluster.DeepCopy()

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

	// Monitor state till RayCluster is ready
	// TODO(#605): Remove after introducing Federated Watcher for watching RayCluster
	clusterStatus, err := r.getClusterStatus(ctx, logger, assignedCluster, &rayCluster)
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("cluster not found on remote cluster, requeue")
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		logger.Error(err, "failed to get cluster status")
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	if err := r.applyRayClusterStatus(&rayCluster, clusterStatus, logger, &res); err != nil {
		logger.Error(err, "failed to apply cluster status")
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	// Update the RayCluster status if any changes occurred
	if !reflect.DeepEqual(originalRayCluster.Status, rayCluster.Status) {
		if err := jobsutils.UpdateStatusWithRetries(
			ctx, r, &rayCluster,
			func(obj client.Object) {
				cluster := obj.(*v2pb.RayCluster)
				cluster.Status = rayCluster.Status
			},
			&metav1.UpdateOptions{},
		); err != nil {
			logger.Error(err, "failed to update ray cluster status")
			return res, fmt.Errorf("update ray cluster status for %q: %w", req.NamespacedName, err)
		}
	}

	logger.Info("Reconcile finished", "requeueAfter", res.RequeueAfter)
	return res, nil
}

// Register registers the RayCluster controller with the controller manager.
//
// This method configures the controller to watch RayCluster custom resources and
// trigger reconciliation when they are created, updated, or deleted.
//
// Returns an error if controller registration fails.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}).
		Complete(r)
}

// shouldTerminateCluster determines if the cluster should enter termination flow.
//
// Termination is triggered when:
//  1. Spec.Termination is set (user-requested termination)
//  2. Succeeded condition is set to TRUE or FALSE (completion or failure)
//
// Returns true if termination should proceed, false otherwise.
func (r *Reconciler) shouldTerminateCluster(cluster *v2pb.RayCluster) bool {
	shouldTerminate, _ := jobsutils.IsTerminationInfoSet(cluster)
	if shouldTerminate {
		return true
	}

	// If the cluster has been marked as failed/succeeded, then terminate
	success := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
	return success.Status != apipb.CONDITION_STATUS_UNKNOWN
}

// processClusterTermination orchestrates the complete cluster termination workflow.
//
// The termination process consists of three sequential steps:
//  1. Set succeeded condition based on termination type (SUCCEEDED/FAILED)
//  2. Set killing condition to indicate cleanup has started
//  3. Delete cluster resources via federated client
//
// Returns an error if any step fails, causing reconciliation to retry.
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

// setClusterSuccessConditionIfRequired sets the succeeded condition from termination info.
//
// This method maps termination type to condition status:
//   - TERMINATION_TYPE_SUCCEEDED → CONDITION_STATUS_TRUE
//   - TERMINATION_TYPE_FAILED → CONDITION_STATUS_FALSE
//
// The condition is only set if not already set (idempotent operation).
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

// setClusterKillIfRequired marks the cluster for deletion.
//
// The killing condition is set only if not already set and the cluster has not been
// killed yet. This signals that cleanup should begin.
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

// cleanupCluster deletes cluster resources based on lifecycle stage.
//
// Cleanup behavior depends on cluster state:
//   - Not scheduled: Mark as killed immediately
//   - Scheduled but not launched: Mark as killed (no remote resources to delete)
//   - Launched: Delete via federated client, then mark as killed
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

	// Update status to terminated
	// TODO(#605): Mark the cluster as killing and once federated watcher is introduced, the watcher will mark the cluster as killed after RayCluster is terminated in the compute cluster.
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

// enqueueIfRequired adds the cluster to the scheduler queue if needed.
//
// Enqueuing behavior:
//   - First time (Enqueued condition not set): Set condition and enqueue
//   - Already enqueued but not scheduled: Re-enqueue (handles controller restarts)
//   - Already enqueued and scheduled: No action needed
//
// The scheduler queue manages resource allocation across available Kubernetes clusters.
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

// getClusterIfScheduled retrieves the physical cluster assignment if scheduling is complete.
//
// Returns the assigned v2pb.Cluster if:
//  1. Scheduled condition is TRUE
//  2. Status.Assignment.Cluster is set
//  3. Cluster exists in the cluster cache
//
// Returns nil if any of these conditions are not met.
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

// getClusterStatus polls the remote cluster for Ray cluster status.
//
// This method uses the federated client to query the assigned Kubernetes cluster
// for the current state of the Ray cluster.
//
// TODO(#605): Replace polling with federated watcher for real-time status updates.
func (r *Reconciler) getClusterStatus(ctx context.Context, log logr.Logger, assignedKubeCluster *v2pb.Cluster, rayCluster *v2pb.RayCluster) (*matypes.JobClusterStatus, error) {
	// Use the federated client to get the status from the remote cluster
	clusterStatus, err := r.federatedClient.GetJobClusterStatus(ctx, rayCluster, assignedKubeCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster status: %w", err)
	}

	log.V(1).Info("retrieved cluster status",
		"cluster", rayCluster.GetName(),
		"namespace", rayCluster.GetNamespace(),
		"state", clusterStatus.Ray.State)

	return clusterStatus, nil
}

// applyRayClusterStatus updates local status based on remote cluster state.
//
// This method processes the status received from the federated cluster and updates
// the local RayCluster resource accordingly:
//
//   - READY: Mark launched condition as TRUE, stop requeuing
//   - FAILED/UNHEALTHY: Mark succeeded condition as FALSE to trigger termination
//   - Other states: Continue monitoring with requeue
func (r *Reconciler) applyRayClusterStatus(
	rayCluster *v2pb.RayCluster,
	clusterStatus *matypes.JobClusterStatus,
	logger logr.Logger,
	res *ctrl.Result,
) error {
	if clusterStatus == nil || clusterStatus.Ray == nil {
		return fmt.Errorf("received nil cluster status")
	}

	// Extract state from the typed status
	newState := clusterStatus.Ray.State

	// Update cluster state
	rayCluster.Status.State = newState
	res.RequeueAfter = requeueAfter

	// Extract reason for condition updates
	reasonStr := clusterStatus.Reason
	// Handle state-specific logic and condition updates
	succeededCond := jobsutils.GetCondition(&rayCluster.Status.StatusConditions, SucceededCondition, rayCluster.Generation)
	launchedCond := jobsutils.GetCondition(&rayCluster.Status.StatusConditions, LaunchedCondition, rayCluster.Generation)

	switch newState {
	case v2pb.RAY_CLUSTER_STATE_READY:
		logger.Info("cluster is ready", "state", newState, "reason", reasonStr)
		jobsutils.UpdateCondition(launchedCond, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_TRUE,
			Generation: rayCluster.Generation,
			Reason:     "ClusterReady",
		})
		res.RequeueAfter = time.Duration(0)

	case v2pb.RAY_CLUSTER_STATE_FAILED:
		logger.Error(nil, "cluster has failed, marking for termination",
			"state", newState,
			"reason", reasonStr,
		)

		// Mark succeeded condition as false to trigger termination
		if reasonStr == "" {
			reasonStr = "ClusterFailed"
		}
		jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: rayCluster.Generation,
			Reason:     reasonStr,
		})

	case v2pb.RAY_CLUSTER_STATE_UNHEALTHY:
		logger.Info("cluster is unhealthy, marking for termination",
			"state", newState,
			"reason", reasonStr,
		)
		// Mark succeeded condition as false to trigger termination
		if reasonStr == "" {
			reasonStr = "ClusterUnhealthy"
		}
		jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: rayCluster.Generation,
			Reason:     reasonStr,
		})

	case v2pb.RAY_CLUSTER_STATE_UNKNOWN:
		logger.Info("cluster state is unknown, will continue monitoring",
			"state", newState,
			"reason", reasonStr)

	default:
		logger.Info("cluster in transitional state, continuing to monitor",
			"state", newState.String(),
			"reason", reasonStr)
	}

	return nil
}
