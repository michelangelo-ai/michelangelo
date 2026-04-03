package job

import (
	"context"
	"fmt"
	"time"

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

const _eventHandlerTimeout = 10 * time.Second

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
		Scope: r.metricsScope,
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
		log.Error(err, "could not fetch the global ray job")
		return
	}

	// Skip updates if the global RayJob is immutable or being deleted.
	if utils.IsImmutable(&rayJob) {
		log.V(1).Info("skipping status update for immutable job")
		return
	}
	if rayJob.GetDeletionTimestamp() != nil {
		log.V(1).Info("skipping status update for job being deleted")
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

	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current v2pb.RayJob
		if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
			return getErr
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
		log.Error(err, "could not fetch the global ray job")
		return
	}

	killing := jobsutils.GetCondition(&rayJob.Status.StatusConditions, constants.KillingCondition, rayJob.Generation)

	if killing.Status != apipb.CONDITION_STATUS_TRUE {
		// Job was deleted externally without going through the controller.
		if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var current v2pb.RayJob
			if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
				return getErr
			}
			succeeded := jobsutils.GetCondition(&current.Status.StatusConditions, constants.SucceededCondition, current.Generation)
			jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Reason:     constants.ClusterKilled,
				Generation: current.Generation,
				Message:    fmt.Sprintf("ray job was deleted externally, status: %+v", local.Status.JobStatus),
			})
			killed := jobsutils.GetCondition(&current.Status.StatusConditions, constants.KilledCondition, current.Generation)
			jobsutils.UpdateCondition(killed, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: current.Generation,
			})
			return r.Status().Update(ctx, &current)
		}); err != nil {
			log.Error(err, "failed to update status on external delete")
			return
		}
		log.Info("job externally deleted, marked as killed")
		return
	}

	// Expected deletion — killing was in progress.
	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current v2pb.RayJob
		if getErr := r.Get(ctx, types.NamespacedName{Namespace: projectName, Name: local.Name}, &current); getErr != nil {
			return getErr
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
		return r.Status().Update(ctx, &current)
	}); err != nil {
		log.Error(err, "failed to update status on expected delete")
		return
	}
	log.Info("job killed successfully")
}
