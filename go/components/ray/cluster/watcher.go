package cluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// getFederatedWatcher creates a federated watcher for the RayCluster controller.
//
// It watches two resources on every ready compute cluster:
//   - the head Pod, to populate RayCluster.Status.HeadNode and PodErrors, and
//   - the KubeRay RayCluster, to propagate cluster State/conditions.
//
// This mirrors the internal ray/controller.go watcher, retargeted onto the OSS
// RayCluster CRD (in OSS the head node / pod errors live on RayClusterStatus).
func (r *Reconciler) getFederatedWatcher() watch.FederatedWatcher {
	return watch.NewFederatedWatcher(watch.FederatedWatcherParams{
		ClusterCache:    r.clusterCache,
		FederatedClient: r.federatedClient,
		Logger:          r.logger.WithValues("resource", "raycluster"),
		WatcherParams: []*jobsclient.WatcherParams{
			{
				ResourceName: corev1.ResourcePods.String(),
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.RayNodeLabelKey:      constants.IsRayNodeValue,
						constants.OwnerServiceLabelKey: constants.MAOwnerServiceLabelValue,
						// MA controller manager runs multiple environments; only handle
						// pods started by this environment.
						constants.JobControlPlaneEnvKey: r.env.RuntimeEnvironment,
					},
				},
				ResourceEventHandler: cache.ResourceEventHandlerFuncs{
					AddFunc:    r.podAddEventHandler,
					UpdateFunc: r.podUpdateEventHandler,
					DeleteFunc: r.podDeleteEventHandler,
				},
				Namespace: k8sengine.RayLocalNamespace,
				ObjType:   &corev1.Pod{},
			},
			{
				ResourceName: constants.KubeRayResource,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.OwnerServiceLabelKey:  constants.MAOwnerServiceLabelValue,
						constants.JobControlPlaneEnvKey: r.env.RuntimeEnvironment,
					},
				},
				ResourceEventHandler: cache.ResourceEventHandlerFuncs{
					AddFunc:    r.rayClusterAddEventHandler,
					UpdateFunc: r.rayClusterUpdateEventHandler,
					DeleteFunc: r.rayClusterDeleteEventHandler,
				},
				Namespace: k8sengine.RayLocalNamespace,
				ObjType:   &rayv1.RayCluster{},
			},
		},
		Scope: r.metricsScope,
	})
}

// Pod event handlers — own RayCluster.Status.HeadNode and PodErrors.

func (r *Reconciler) podAddEventHandler(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	r.handlePodEvent(pod)
}

func (r *Reconciler) podUpdateEventHandler(_, newObj interface{}) {
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	r.handlePodEvent(pod)
}

// handlePodEvent splits an add/update into the two things the pod watcher owns.
//
// Head-node connection details need a running head pod. Pod errors do not, and
// must not wait for one: a container wedged in ImagePullBackOff or
// CrashLoopBackOff keeps its pod alive indefinitely, so podDeleteEventHandler
// never fires and the failure would otherwise never reach the CR. Worker pods
// are checked too -- the informer already watches every Ray node, and the
// delete path has always handled both.
func (r *Reconciler) handlePodEvent(pod *corev1.Pod) {
	// We re-inspect even if already recorded to handle the case where we missed
	// the original event.
	if pod.Status.Phase == corev1.PodRunning && jobsutils.IsRayHeadNode(pod) {
		r.podEventHandler(pod)
	}
	r.podErrorEventHandler(pod)
}

// podErrorEventHandler records a container-level failure on a pod that is still
// alive. It uses the container-only extraction: a pod that is merely starting
// up reports ContainersReady=False, which is progress, not an error.
func (r *Reconciler) podErrorEventHandler(pod *corev1.Pod) {
	podError := jobsutils.GetContainerErrorFromPodStatus(pod, isRayContainer)
	if podError == nil {
		// A pod the scheduler could not place has no container statuses to
		// inspect, and it is never deleted either, so the delete path will not
		// pick it up later. Its PodScheduled condition is the only record of
		// why the cluster is not coming up.
		podError = jobsutils.GetSchedulingErrorFromPodStatus(pod)
	}
	if podError == nil {
		return
	}
	r.recordPodError(pod, podError, "podErrorEventHandler")
}

// isRayContainer matches the containers whose failures belong to the cluster.
//
// The ray containers are named by the job author, so there is no fixed name to
// match on. The only container michelangelo names itself is the log-collector
// sidecar it appends to every pod template, so everything that is not the
// collector is the author's workload and its failures are the cluster's.
func isRayContainer(containerStatus corev1.ContainerStatus) bool {
	return containerStatus.Name != constants.CollectorContainerName
}

// recordPodError merges a pod-level failure onto the owning RayCluster CR.
//
// Both the live path and the delete path funnel through here so the two cannot
// double-count the same pod, and so the mutation is idempotent:
// UpdateStatusWithRetries re-runs its closure on conflict, which an append
// would turn into duplicate entries. The cap on the number of recorded errors
// is enforced by mergePodErrors.
func (r *Reconciler) recordPodError(pod *corev1.Pod, podError *v2pb.PodErrors, fieldManager string) {
	log := r.logger.WithValues("pod_name", pod.Name, "reason", podError.GetReason())

	namespace, err := jobsutils.GetProjectNameFromLabels(pod.Labels)
	if err != nil {
		log.Error(err, "unable to determine namespace of ray cluster - not recording the pod error")
		return
	}
	clusterName, err := getClusterName(pod)
	if err != nil {
		log.Error(err, "unable to get cluster name - not recording the pod error")
		return
	}
	log = log.WithValues("namespace", namespace, "ray_cluster", clusterName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayCluster v2pb.RayCluster
	if err = r.Get(ctx, namespace, clusterName, &metav1.GetOptions{}, &rayCluster); err != nil {
		if utils.IsNotFoundError(err) {
			// The global RayCluster is gone, so there is nothing to record the pod error
			// on. Pods outlive the cluster CR during teardown, so this is expected.
			log.V(1).Info("global ray cluster no longer exists, not recording the pod error")
			return
		}
		log.Error(err, "could not fetch the ray cluster for the pod - not recording the pod error")
		return
	}

	if err = jobsutils.UpdateStatusWithRetries(ctx, r, &rayCluster,
		func(obj client.Object) {
			mergePodErrors(obj.(*v2pb.RayCluster), []*v2pb.PodErrors{podError})
		}, &metav1.UpdateOptions{
			FieldManager: fieldManager,
		}); err != nil {
		log.Error(err, "could not update the ray cluster with the pod error")
	}
}

func (r *Reconciler) podEventHandler(pod *corev1.Pod) {
	log := r.logger.WithValues("pod_name", pod.Name)

	clusterName, err := getClusterName(pod)
	if err != nil {
		log.Error(err, "unable to get cluster name")
		return
	}
	log = log.WithValues("ray_cluster", clusterName)

	// The dynamic port annotations are optional -- a cluster that publishes neither
	// is a valid configuration, not a fault. getPort reports the absence as an error
	// and yields -1, which is recorded verbatim below, so keep this at debug level
	// instead of logging two errors for every head pod event.
	clientPort, err := getClientPort(pod)
	if err != nil {
		log.V(1).Info("no client port published for the head pod", "reason", err.Error())
	}

	jupyterNotebookPort, err := getJupyterNotebookPort(pod)
	if err != nil {
		log.V(1).Info("no jupyter notebook port published for the head pod", "reason", err.Error())
	}

	log.Info("Retrieved head pod info",
		"ray_head_ip", pod.Status.PodIP,
		"ray_head_client_port", clientPort,
		"ray_head_jupyter_notebook_port", jupyterNotebookPort)

	namespace, err := jobsutils.GetProjectNameFromLabels(pod.Labels)
	if err != nil {
		log.Error(err, "unable to determine namespace of ray cluster")
		return
	}
	log = log.WithValues("namespace", namespace)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayCluster v2pb.RayCluster
	if err = r.Get(ctx, namespace, clusterName, &metav1.GetOptions{}, &rayCluster); err != nil {
		if utils.IsNotFoundError(err) {
			log.V(1).Info("global ray cluster no longer exists, ignoring pod event")
			return
		}
		log.Error(err, "could not fetch the ray cluster for the pod")
		return
	}

	// Cache re-sync gives Update events even when the head pod did not change.
	// Pre-check to avoid unnecessary CRD object updates.
	if rayCluster.Status.HeadNode != nil &&
		rayCluster.Status.HeadNode.Ip == pod.Status.PodIP &&
		rayCluster.Status.HeadNode.ClientPort == clientPort &&
		rayCluster.Status.HeadNode.JupyterNotebookPort == jupyterNotebookPort {
		return
	}

	if err = jobsutils.UpdateStatusWithRetries(ctx, r, &rayCluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			cluster.Status.HeadNode = &v2pb.RayHeadNodeInfo{
				Name:                pod.Name,
				Namespace:           pod.Namespace,
				Ip:                  pod.Status.PodIP,
				ClientPort:          clientPort,
				JupyterNotebookPort: jupyterNotebookPort,
			}
		}, &metav1.UpdateOptions{
			FieldManager: "podEventHandler",
		}); err != nil {
		log.Error(err, "could not update head node info for the ray cluster")
	}
}

func (r *Reconciler) podDeleteEventHandler(obj interface{}) {
	// OnDelete can return a DeletedFinalStateUnknown tombstone if the informer
	// missed the final delete. Recover the last-known Pod and continue best-effort.
	var pod *corev1.Pod
	switch v := obj.(type) {
	case *corev1.Pod:
		pod = v
	case cache.DeletedFinalStateUnknown:
		r.logger.Info("Received tombstone delete event for Ray pod", "ray_pod_tombstone_key", v.Key)
		p, ok := v.Obj.(*corev1.Pod)
		if !ok {
			r.logger.Error(fmt.Errorf("could not extract Pod from tombstone, unexpected object type %T", v.Obj), "skipping delete event")
			return
		}
		pod = p
	default:
		r.logger.Error(fmt.Errorf("unexpected object type %T in delete handler", v), "skipping delete event")
		return
	}
	log := r.logger.WithValues("pod_name", pod.Name)

	namespace, err := jobsutils.GetProjectNameFromLabels(pod.Labels)
	if err != nil {
		log.Error(err, "unable to determine namespace of ray cluster - not processing pod delete event further")
		return
	}
	log = log.WithValues("namespace", namespace)

	clusterName, err := getClusterName(pod)
	if err != nil {
		log.Error(err, "unable to get cluster name - not processing pod delete event further")
		return
	}
	log = log.WithValues("ray_cluster", clusterName)

	if pod.Status.Phase == corev1.PodFailed {
		log.Info("pod failed", "reason", pod.Status.Reason, "status_message", pod.Status.Message,
			"container_statuses", pod.Status.ContainerStatuses, "init_container_statuses", pod.Status.InitContainerStatuses)
	}

	// On delete the pod conditions are worth consulting as well: the pod is
	// gone, so ContainersReady=False is a final verdict rather than progress.
	podError := jobsutils.GetErrorFromPodStatus(pod, isRayContainer)
	// If no container errors are found we do not need to update the status.
	if podError == nil {
		return
	}
	r.recordPodError(pod, podError, "podDeleteEventHandler")
}

// KubeRay RayCluster event handlers — own RayCluster.Status.State and conditions.

// rayClusterAddEventHandler handles add events for KubeRay RayCluster resources.
func (r *Reconciler) rayClusterAddEventHandler(obj interface{}) {
	r.rayClusterEventHandler(obj)
}

// rayClusterUpdateEventHandler handles update events for KubeRay RayCluster resources.
func (r *Reconciler) rayClusterUpdateEventHandler(_, newObj interface{}) {
	r.rayClusterEventHandler(newObj)
}

// rayClusterDeleteEventHandler handles delete events for KubeRay RayCluster resources.
func (r *Reconciler) rayClusterDeleteEventHandler(obj interface{}) {
	// Recover the last-known RayCluster from a tombstone if the informer missed
	// the final delete; the project namespace is only available from its labels.
	var local *rayv1.RayCluster
	switch v := obj.(type) {
	case *rayv1.RayCluster:
		local = v
	case cache.DeletedFinalStateUnknown:
		r.logger.Info("received tombstone delete event for ray cluster", "key", v.Key)
		c, ok := v.Obj.(*rayv1.RayCluster)
		if !ok {
			r.logger.Error(fmt.Errorf("could not extract RayCluster from tombstone, unexpected object type %T", v.Obj), "skipping delete event")
			return
		}
		local = c
	default:
		r.logger.Error(fmt.Errorf("unexpected object type %T in delete handler", v), "skipping delete event")
		return
	}
	log := r.logger.WithValues("ray_cluster", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the ray cluster")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var globalCluster v2pb.RayCluster
	if err := r.Get(ctx, projectName, local.Name, &metav1.GetOptions{}, &globalCluster); err != nil {
		if utils.IsNotFoundError(err) {
			// The global RayCluster is gone: the ingester moved a terminal cluster to
			// metadata storage, or it was deleted outright. The KubeRay object outlives
			// it during teardown, so its trailing events are expected here.
			log.V(1).Info("global ray cluster no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global ray cluster")
		return
	}

	// A frozen cluster has already reached a terminal outcome and is waiting on the
	// ingester to archive it; writing to it would restart reconciliation for no gain.
	if utils.IsImmutable(&globalCluster) {
		log.Info("skipping delete event for immutable ray cluster")
		return
	}

	killing := jobsutils.GetCondition(&globalCluster.Status.StatusConditions, KillingCondition, globalCluster.Generation)

	if killing.Status != apipb.CONDITION_STATUS_TRUE {
		// Cluster was deleted externally without going through the controller. The
		// backing resource is already confirmed gone, so record the complete terminal
		// condition set in a single write (State=FAILED, Succeeded=FALSE,
		// Killing=FALSE, Killed=TRUE) — the same set the polling path wrote when it
		// found the cluster missing on the remote compute cluster. Killing=FALSE
		// matters beyond bookkeeping: isClusterFullyTerminal requires it, and
		// cleanupCluster would otherwise issue a pointless DeleteJobCluster.
		if err := r.markClusterFailed(ctx, &globalCluster,
			constants.ClusterKilled,
			fmt.Sprintf("ray cluster was deleted externally, state: %+v", local.Status.State),
		); err != nil {
			log.Error(err, "failed to update status on external delete")
			return
		}
		if err := r.markImmutableIfTerminal(ctx, &globalCluster); err != nil {
			log.Error(err, "failed to mark cluster immutable after external delete")
			return
		}
		log.Info("cluster externally deleted, marked as killed")
		return
	}

	// Expected deletion — killing was in progress.
	if err := jobsutils.UpdateStatusWithRetries(ctx, r, &globalCluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			killingCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, KillingCondition, cluster.Generation)
			jobsutils.UpdateCondition(killingCond, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Generation: cluster.Generation,
			})
			killedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, KilledCondition, cluster.Generation)
			jobsutils.UpdateCondition(killedCond, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: cluster.Generation,
			})
		}, &metav1.UpdateOptions{
			FieldManager: "rayClusterDeleteEventHandler",
		}); err != nil {
		log.Error(err, "failed to update status on expected delete")
		return
	}
	// Reconcile's termination branch freezes the cluster via finalizeIfTerminal once it
	// converges; the delete event is the other path that completes a kill, so it has to
	// freeze the object too, otherwise the ingester never archives it and the cluster
	// reconciles forever.
	if err := r.markImmutableIfTerminal(ctx, &globalCluster); err != nil {
		log.Error(err, "failed to mark cluster immutable after kill")
		return
	}
	log.Info("cluster killed successfully")
}

// rayClusterEventHandler syncs KubeRay RayCluster state to the global RayCluster CR.
func (r *Reconciler) rayClusterEventHandler(obj interface{}) {
	local, ok := obj.(*rayv1.RayCluster)
	if !ok {
		// Ignore events from ill-formed objects.
		return
	}
	log := r.logger.WithValues("ray_cluster", local.Name)

	projectName, err := jobsutils.GetProjectNameFromLabels(local.Labels)
	if err != nil {
		log.Error(err, "could not find the project name of the ray cluster")
		return
	}
	log = log.WithValues("namespace", projectName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var globalCluster v2pb.RayCluster
	if err := r.Get(ctx, projectName, local.Name, &metav1.GetOptions{}, &globalCluster); err != nil {
		if utils.IsNotFoundError(err) {
			// The global RayCluster is gone: the ingester moved a terminal cluster to
			// metadata storage, or it was deleted outright. The KubeRay object outlives
			// it during teardown, so its trailing events are expected here.
			log.V(1).Info("global ray cluster no longer exists, ignoring event")
			return
		}
		log.Error(err, "could not fetch the global ray cluster")
		return
	}

	// Skip updates if the global RayCluster is immutable or being deleted.
	if utils.IsImmutable(&globalCluster) {
		log.V(1).Info("skipping status update for immutable cluster")
		return
	}
	if globalCluster.GetDeletionTimestamp() != nil {
		log.V(1).Info("skipping status update for cluster being deleted")
		return
	}

	// Derive the globally-meaningful parts of the status from the compute-cluster
	// object. The mapper owns that translation — it knows the log-URL template,
	// the compute-cluster Ray namespace, and how to read a reason and pod errors
	// out of KubeRay's conditions — so surfacing what it returns keeps the
	// event-driven path reporting exactly what the polling path used to.
	var (
		logURL             string
		reason             string
		conditionPodErrors []*v2pb.PodErrors
	)
	if globalStatus, err := r.mapper.MapLocalClusterStatusToGlobal(local); err != nil {
		log.Error(err, "could not map the local ray cluster status, falling back to state only")
	} else if globalStatus != nil {
		reason = globalStatus.Reason
		if globalStatus.Ray != nil {
			logURL = globalStatus.Ray.LogUrl
			conditionPodErrors = globalStatus.Ray.PodErrors
		}
	}

	newState := mapKubeRayClusterState(local.Status.State)

	// Suspension is visible in spec/conditions before status.state catches up
	// (e.g. a cluster suspended by its admission webhook at creation still has
	// state ""); report SUSPENDED for the whole window. Mirrors
	// k8sengine.convertRayV1ClusterStatusToV2.
	if isSuspended(local) {
		newState = v2pb.RAY_CLUSTER_STATE_SUSPENDED
	}

	// Cache re-sync gives Update events even when the local cluster did not
	// change. Exit early if global already reflects the local state to avoid
	// unnecessary CRD writes.
	// The log URL clause keeps a cluster that went ready under an older build
	// (or before log persistence was configured) from being skipped forever.
	if globalCluster.Status.State == newState && newState == v2pb.RAY_CLUSTER_STATE_READY &&
		(logURL == "" || globalCluster.Status.LogUrl != "") {
		launchedCond := jobsutils.GetCondition(&globalCluster.Status.StatusConditions, LaunchedCondition, globalCluster.Generation)
		if launchedCond.GetStatus() == apipb.CONDITION_STATUS_TRUE {
			log.V(1).Info("ray cluster already ready, skipping update")
			return
		}
	}

	log.Info("ray cluster event", "kuberay_state", local.Status.State, "mapped_state", newState)

	if err := jobsutils.UpdateStatusWithRetries(ctx, r, &globalCluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			cluster.Status.State = newState
			if logURL != "" {
				cluster.Status.LogUrl = logURL
			}
			mergePodErrors(cluster, conditionPodErrors)

			switch newState {
			case v2pb.RAY_CLUSTER_STATE_READY:
				launchedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, LaunchedCondition, cluster.Generation)
				jobsutils.UpdateCondition(launchedCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Generation: cluster.Generation,
					Reason:     "ClusterReady",
				})
				// A cluster that was waiting for admission is admitted now. Only
				// flip an existing Queued condition: clusters that were never
				// suspended should not grow one.
				for _, cond := range cluster.Status.StatusConditions {
					if cond.GetType() == QueuedCondition && cond.GetStatus() == apipb.CONDITION_STATUS_TRUE {
						jobsutils.UpdateCondition(cond, jobsutils.ConditionUpdateParams{
							Status:     apipb.CONDITION_STATUS_FALSE,
							Generation: cluster.Generation,
							Reason:     "ClusterAdmitted",
						})
					}
				}
			case v2pb.RAY_CLUSTER_STATE_FAILED:
				succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
				jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Generation: cluster.Generation,
					Reason:     reasonOr(reason, "ClusterFailed"),
				})
			case v2pb.RAY_CLUSTER_STATE_UNHEALTHY:
				succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
				jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Generation: cluster.Generation,
					Reason:     reasonOr(reason, "ClusterUnhealthy"),
				})
			case v2pb.RAY_CLUSTER_STATE_UNKNOWN:
				// If the cluster is in an unknown state but we have already recorded
				// terminal pod errors, treat it as failed to trigger termination.
				if jobsutils.HasTerminalPodErrors(cluster.Status.PodErrors) {
					cluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
					succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
					jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
						Status:     apipb.CONDITION_STATUS_FALSE,
						Generation: cluster.Generation,
						Reason:     reasonOr(reason, "ClusterFailedWithPodErrors"),
					})
				}
			case v2pb.RAY_CLUSTER_STATE_SUSPENDED:
				// Suspension (RayCluster.spec.suspend, e.g. Kueue admission
				// gating) is non-terminal: pods are intentionally absent until
				// the cluster is unsuspended, so keep monitoring without
				// touching the Succeeded condition. Deliberately no
				// terminal-pod-error check here — while suspended, pod-level
				// signals carry no meaning. Mirrors #1700's controller handling.
				// Surface the wait as a Queued condition so callers can tell
				// "held for admission" apart from "launching".
				queuedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, QueuedCondition, cluster.Generation)
				jobsutils.UpdateCondition(queuedCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Generation: cluster.Generation,
					Reason:     "AwaitingAdmission",
					Message:    reason,
				})
			}
		}, &metav1.UpdateOptions{
			FieldManager: "rayClusterEventHandler",
		}); err != nil {
		log.Error(err, "failed to update global ray cluster status")
	}
}

// reasonOr returns the mapper-derived reason, falling back to a generic one when
// KubeRay's conditions carry nothing more specific.
func reasonOr(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

// mergePodErrors upserts pod errors onto the global status.
//
// Entries are keyed by name, which is the condition type ("ReplicaFailure")
// for errors derived from the KubeRay RayCluster and the pod name for those
// derived from a Pod. The two therefore never collide, and repeated events —
// cache re-syncs fire every _reSyncPeriod, and a wedged pod is re-reported on
// every status change — update in place rather than growing the list. That
// also keeps the mutation idempotent under UpdateStatusWithRetries, which
// re-runs its closure on conflict. The list is bounded by two errors per
// expected pod; at the bound, existing entries can still be refreshed but new
// ones are dropped.
func mergePodErrors(cluster *v2pb.RayCluster, podErrors []*v2pb.PodErrors) {
	if len(podErrors) == 0 {
		return
	}
	maxPodErrorLength := 2 * (1 + jobsutils.NumRayWorkers(cluster))
	indexByName := make(map[string]int, len(cluster.Status.PodErrors))
	for i, podError := range cluster.Status.PodErrors {
		indexByName[podError.GetName()] = i
	}
	for _, podError := range podErrors {
		if i, ok := indexByName[podError.GetName()]; ok {
			// Killing a failed cluster deletes its pods, and the kubelet then
			// reports every container as ContainerStatusUnknown. Overwriting on
			// that would leave the CR describing only the teardown -- "the
			// container could not be located when the pod was terminated" --
			// and erase the ImagePullBackOff or CrashLoopBackOff that actually
			// caused the failure, which is the one thing a user reading a dead
			// cluster needs.
			if jobsutils.IsPostMortemPodError(podError) && !jobsutils.IsPostMortemPodError(cluster.Status.PodErrors[i]) {
				continue
			}
			cluster.Status.PodErrors[i] = podError
			continue
		}
		if len(cluster.Status.PodErrors) >= maxPodErrorLength {
			return
		}
		indexByName[podError.GetName()] = len(cluster.Status.PodErrors)
		cluster.Status.PodErrors = append(cluster.Status.PodErrors, podError)
	}
}

// mapKubeRayClusterState maps a KubeRay v1 ClusterState to our internal RayClusterState.
// It mirrors k8sengine.getRayClusterStateFromKubeRayState (KubeRay has no exported
// "unhealthy" constant, hence the string literal).
func mapKubeRayClusterState(state rayv1.ClusterState) v2pb.RayClusterState {
	switch state {
	case rayv1.Ready:
		return v2pb.RAY_CLUSTER_STATE_READY
	case rayv1.Failed:
		return v2pb.RAY_CLUSTER_STATE_FAILED
	case "unhealthy":
		return v2pb.RAY_CLUSTER_STATE_UNHEALTHY
	case rayv1.Suspended:
		return v2pb.RAY_CLUSTER_STATE_SUSPENDED
	default:
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	}
}

// KubeRay writes these condition types while suspending/resuming a cluster
// (RayCluster.spec.suspend). The vendored ray-operator API (v1.2.2) predates
// their constants, so they are string-matched here. Mirrors k8sengine.
const (
	rayClusterSuspendingConditionType = "RayClusterSuspending"
	rayClusterSuspendedConditionType  = "RayClusterSuspended"
)

// isSuspended reports whether the cluster is suspended or being suspended
// (spec.suspend=true, e.g. set by a queueing/admission controller such as
// Kueue). Spec intent, reported state, and conditions are all checked because
// status.state lags spec.suspend when a cluster is suspended at creation.
// Mirrors k8sengine.isSuspended so the watcher and the mapper agree.
func isSuspended(rc *rayv1.RayCluster) bool {
	if rc.Spec.Suspend != nil && *rc.Spec.Suspend {
		return true
	}
	if rc.Status.State == rayv1.Suspended {
		return true
	}
	for _, cond := range rc.Status.Conditions {
		if cond.Status != metav1.ConditionTrue {
			continue
		}
		if cond.Type == rayClusterSuspendingConditionType || cond.Type == rayClusterSuspendedConditionType {
			return true
		}
	}
	return false
}

// Helper functions for extracting head pod information.

func getClusterName(pod *corev1.Pod) (string, error) {
	name, ok := pod.Labels[constants.RayClusterNameLabelKey]
	if ok {
		return name, nil
	}
	return "", fmt.Errorf("could not find out the cluster name from pod labels: %v", pod.Labels)
}

func getPort(portName string, pod *corev1.Pod) (int32, error) {
	portAnnotation, ok := pod.Annotations[fmt.Sprintf("%s%s", constants.DynamicPortAnnotationKeyPrefix, portName)]
	if !ok {
		return -1, fmt.Errorf("port not found in annotations: %s", portName)
	}

	port, err := strconv.Atoi(portAnnotation)
	if err != nil {
		return -1, fmt.Errorf("port is not an integer: %s", portAnnotation)
	}

	return int32(port), nil
}

func getClientPort(pod *corev1.Pod) (int32, error) {
	return getPort(constants.RayClientPort, pod)
}

func getJupyterNotebookPort(pod *corev1.Pod) (int32, error) {
	return getPort(constants.JupyterNotebookPort, pod)
}
