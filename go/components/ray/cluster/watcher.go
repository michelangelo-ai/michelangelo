package cluster

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/uber-go/tally"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/watch"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	_eventHandlerTimeout = 10 * time.Second
	// Counts handler status writes that gave up, tagged by handler.
	_handlerWriteDroppedMetricName = "handler_write_dropped"
)

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

// watcherScope namespaces the watcher's metrics; the RayJob controller shares
// the root scope and watches its own resources.
func (r *Reconciler) watcherScope() tally.Scope {
	return r.metricsScope.SubScope("raycluster")
}

// updateStatusFromHandler writes a status change from a watcher handler. A
// handler has no requeue: a write that gives up loses its event until the next
// re-sync, and for a delete for good. So it retries for longer than the
// reconciler does -- long enough for the cache it reads from to catch up with
// another writer's update -- and a write it drops anyway is counted.
func (r *Reconciler) updateStatusFromHandler(ctx context.Context, cluster *v2pb.RayCluster,
	applyUpdates func(obj client.Object), handler string,
) error {
	err := jobsutils.UpdateStatusWithBackoff(ctx, r, cluster, applyUpdates,
		&metav1.UpdateOptions{FieldManager: handler}, retry.DefaultBackoff)
	if err != nil {
		r.watcherScope().Tagged(map[string]string{"handler": handler}).Counter(_handlerWriteDroppedMetricName).Inc(1)
	}
	return err
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
		// A pod that has come good -- its image pulled on a retry, its container
		// restarted after an OOM kill -- must not keep the error it reported on
		// the way: the entry would linger on a healthy cluster and still count
		// towards failing it.
		if isPodRunningAndReady(pod) {
			r.clearPodError(pod)
		}
		return
	}
	r.recordPodError(pod, podError, "podErrorEventHandler", false)
}

// clearPodError drops the entry a recovered pod left on the owning RayCluster.
// It writes only when there is such an entry, since every healthy pod comes
// through here on each re-sync.
func (r *Reconciler) clearPodError(pod *corev1.Pod) {
	log := r.logger.WithValues("pod_name", pod.Name)

	namespace, err := jobsutils.GetProjectNameFromLabels(pod.Labels)
	if err != nil {
		log.Error(err, "unable to determine namespace of ray cluster - not clearing the pod error")
		return
	}
	clusterName, err := getClusterName(pod)
	if err != nil {
		log.Error(err, "unable to get cluster name - not clearing the pod error")
		return
	}
	log = log.WithValues("namespace", namespace, "ray_cluster", clusterName)

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayCluster v2pb.RayCluster
	if err = r.Get(ctx, namespace, clusterName, &metav1.GetOptions{}, &rayCluster); err != nil {
		if utils.IsNotFoundError(err) {
			log.V(1).Info("global ray cluster no longer exists, nothing to clear")
			return
		}
		log.Error(err, "could not fetch the ray cluster for the pod - not clearing the pod error")
		return
	}
	if !removeRecoveredPodError(rayCluster.DeepCopy(), pod.Name) {
		return
	}

	if err = r.updateStatusFromHandler(ctx, &rayCluster,
		func(obj client.Object) {
			removeRecoveredPodError(obj.(*v2pb.RayCluster), pod.Name)
		}, "podErrorEventHandler"); err != nil {
		log.Error(err, "could not clear the recovered pod's error from the ray cluster")
		return
	}
	log.Info("cleared the error of a recovered pod")
}

// removeRecoveredPodError removes podName's entry from cluster and reports
// whether there was one. Post-mortem entries describe a pod that is gone, so a
// live pod of the same name has not cleared them. Nothing is removed once
// termination has begun: by then the recorded errors are the reason the
// cluster ended.
func removeRecoveredPodError(cluster *v2pb.RayCluster, podName string) bool {
	if isFrozenOrDeleting(cluster) || isClusterTerminating(cluster) {
		return false
	}
	for i, podError := range cluster.Status.PodErrors {
		if podError.GetName() == podName && !jobsutils.IsPostMortemPodError(podError) {
			cluster.Status.PodErrors = slices.Delete(cluster.Status.PodErrors, i, i+1)
			return true
		}
	}
	return false
}

// isPodRunningAndReady reports whether pod is running and passing its
// readiness checks.
func isPodRunningAndReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// isFrozenOrDeleting reports whether the watcher must leave cluster alone: it is
// frozen awaiting the ingester, or its CR is being deleted.
func isFrozenOrDeleting(cluster *v2pb.RayCluster) bool {
	return utils.IsImmutable(cluster) || cluster.GetDeletionTimestamp() != nil
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
// double-count the same pod, and so the mutation is idempotent: the status
// write re-runs its closure on conflict, which an append would turn into
// duplicate entries. The cap on the number of recorded errors is enforced by
// mergePodErrors. skipIfTerminating drops an error that only means something
// while the cluster is meant to be running.
func (r *Reconciler) recordPodError(pod *corev1.Pod, podError *v2pb.PodErrors, fieldManager string, skipIfTerminating bool) {
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
	acceptsPodError := func(cluster *v2pb.RayCluster) bool {
		return !isFrozenOrDeleting(cluster) && !(skipIfTerminating && isClusterTerminating(cluster))
	}
	if !acceptsPodError(&rayCluster) {
		log.V(1).Info("ray cluster no longer takes this pod error, not recording it")
		return
	}

	if err = r.updateStatusFromHandler(ctx, &rayCluster,
		func(obj client.Object) {
			// The cluster may have moved on since the read above.
			if cluster := obj.(*v2pb.RayCluster); acceptsPodError(cluster) {
				mergePodErrors(cluster, []*v2pb.PodErrors{podError})
			}
		}, fieldManager); err != nil {
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
	if isFrozenOrDeleting(&rayCluster) {
		log.V(1).Info("ray cluster is frozen or being deleted, ignoring pod event")
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

	if err = r.updateStatusFromHandler(ctx, &rayCluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			if isFrozenOrDeleting(cluster) {
				return
			}
			cluster.Status.HeadNode = &v2pb.RayHeadNodeInfo{
				Name:                pod.Name,
				Namespace:           pod.Namespace,
				Ip:                  pod.Status.PodIP,
				ClientPort:          clientPort,
				JupyterNotebookPort: jupyterNotebookPort,
			}
		}, "podEventHandler"); err != nil {
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

	podError := jobsutils.GetContainerErrorFromPodStatus(pod, isRayContainer)
	fromConditions := false
	if podError == nil && !rayContainersExitedCleanly(pod) {
		// With no container to blame, the pod conditions can still say why the
		// pod went away -- it was preempted, or never placed. A pod whose ray
		// containers all exited 0 had nothing go wrong, though, and neither did
		// one whose only complaint is the ContainersReady=False every deleted
		// pod ends with. Once the cluster is terminating, every pod is going
		// away for the same, known reason, so recordPodError skips these.
		podError = jobsutils.GetErrorFromPodStatus(pod, isRayContainer)
		if podError != nil && normalPodDeletionReasons[podError.GetReason()] {
			podError = nil
		}
		fromConditions = true
	}
	// If no errors are found we do not need to update the status.
	if podError == nil {
		return
	}
	r.recordPodError(pod, podError, "podDeleteEventHandler", fromConditions)
}

// normalPodDeletionReasons are the ContainersReady=False reasons the kubelet
// gives every pod it stops, whether on a teardown, a Ray autoscaler scale-down,
// or KubeRay replacing a pod. Recorded as errors they would pile up on a healthy
// cluster until the cap, crowding out the ones that matter.
var normalPodDeletionReasons = map[string]bool{
	"ContainersNotReady": true,
	"PodCompleted":       true,
}

// rayContainersExitedCleanly reports whether pod ran its ray containers to a
// clean exit, i.e. it has some and every one of them terminated with code 0.
func rayContainersExitedCleanly(pod *corev1.Pod) bool {
	exited := false
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !isRayContainer(containerStatus) {
			continue
		}
		if terminated := containerStatus.State.Terminated; terminated == nil || terminated.ExitCode != 0 {
			return false
		}
		exited = true
	}
	return exited
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

	// Killed=TRUE means the kill is already recorded -- cleanupCluster does that
	// right after it deletes the KubeRay object -- so this delete is its echo.
	// Deciding anything here would race that write: a cleanup that is visible
	// before its freeze would read as an external delete and turn a TERMINATED
	// cluster FAILED. All that can be outstanding is the freeze.
	if conditionStatus(&globalCluster, KilledCondition) == apipb.CONDITION_STATUS_TRUE {
		if err := r.markImmutableIfTerminal(ctx, &globalCluster); err != nil {
			log.Error(err, "failed to mark cluster immutable after kill")
			return
		}
		log.V(1).Info("ray cluster deleted after its kill was recorded")
		return
	}

	if conditionStatus(&globalCluster, KillingCondition) != apipb.CONDITION_STATUS_TRUE {
		// Only a launched cluster has a KubeRay object of its own. A delete for an
		// unlaunched one belongs to something else, such as a same-named
		// predecessor's object that is only now going away.
		if conditionStatus(&globalCluster, LaunchedCondition) != apipb.CONDITION_STATUS_TRUE {
			log.Info("ignoring delete event for a ray cluster that was never launched")
			return
		}
		// Cluster was deleted externally without going through the controller. The
		// backing resource is already confirmed gone, so record the complete terminal
		// condition set in a single write (State=FAILED, Succeeded=FALSE unless an
		// outcome was already decided, Killing=FALSE, Killed=TRUE) — the same set
		// the safety net writes when it finds the cluster missing on the remote
		// compute cluster. Killing=FALSE matters beyond bookkeeping:
		// isClusterFullyTerminal requires it, and cleanupCluster would otherwise
		// issue a pointless DeleteJobCluster.
		if err := r.updateStatusFromHandler(ctx, &globalCluster,
			markFailedUpdate(constants.ClusterKilled,
				fmt.Sprintf("ray cluster was deleted externally, state: %+v", local.Status.State)),
			"rayClusterDeleteEventHandler",
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
	if err := r.updateStatusFromHandler(ctx, &globalCluster,
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
		}, "rayClusterDeleteEventHandler"); err != nil {
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
	// the compute-cluster Ray namespace, how KubeRay's state maps onto ours
	// (including a suspension that spec/conditions show before status.state
	// catches up), and how to read a reason and pod errors out of KubeRay's
	// conditions — so surfacing what it returns keeps the event-driven path
	// reporting exactly what the polling path used to.
	globalStatus, err := r.mapper.MapLocalClusterStatusToGlobal(local)
	if err != nil || globalStatus == nil || globalStatus.Ray == nil {
		log.Error(err, "could not map the local ray cluster status")
		return
	}
	newState := globalStatus.Ray.State

	// Cache re-sync gives Update events even when the local cluster did not
	// change, and a terminating cluster only takes pod errors and the log URL
	// from here on. Apply the observation to a copy first and skip the write when
	// it changes nothing. applyClusterObservation runs again on the object being
	// written, since that can differ from this read.
	if !applyClusterObservation(globalCluster.DeepCopy(), globalStatus) {
		log.V(1).Info("global ray cluster already reflects the ray cluster, skipping update")
		return
	}

	log.Info("ray cluster event", "kuberay_state", local.Status.State, "mapped_state", newState)

	if err := r.updateStatusFromHandler(ctx, &globalCluster,
		func(obj client.Object) {
			applyClusterObservation(obj.(*v2pb.RayCluster), globalStatus)
		}, "rayClusterEventHandler"); err != nil {
		log.Error(err, "failed to update global ray cluster status")
	}
}

// applyClusterObservation folds one observation of the compute-cluster
// RayCluster, as mapped by the k8sengine mapper, into cluster's status, and
// reports whether that changed anything.
//
// This is the cluster state machine, and the only copy of it:
// rayClusterEventHandler applies every KubeRay event through it, and Reconcile's
// safety net applies what GetJobClusterStatus returns, so the event path and the
// level-triggered path cannot disagree. It has to stay pure and idempotent:
// the status write re-runs it on a re-fetched object after a conflict,
// and the safety net applies it to a copy first to skip writes that would change
// nothing.
func applyClusterObservation(cluster *v2pb.RayCluster, observed *matypes.JobClusterStatus) bool {
	if observed == nil || observed.Ray == nil || isFrozenOrDeleting(cluster) {
		return false
	}
	changed := false
	if logURL := observed.Ray.LogUrl; logURL != "" && cluster.Status.LogUrl != logURL {
		cluster.Status.LogUrl = logURL
		changed = true
	}

	// Once termination has begun Reconcile owns State and the conditions. An
	// event read off the informer's backlog after cleanupCluster's write would
	// otherwise turn a TERMINATED cluster READY again or flip a decided
	// Succeeded, and the freeze archives whatever it finds. Pod errors are still
	// worth keeping, but only merged: the recorded ones are now the reason the
	// cluster ended.
	if isClusterTerminating(cluster) {
		return mergePodErrors(cluster, observed.Ray.PodErrors) || changed
	}

	// KubeRay's conditions describe the cluster as it is now, so the entries
	// read from them are replaced rather than merged: one KubeRay has since
	// cleared -- a ReplicaFailure it retried past, a HeadPodReady=False from
	// before the head pod came up -- must not linger, let alone count towards
	// failing the cluster.
	if replaceConditionPodErrors(cluster, observed.Ray.PodErrors) {
		changed = true
	}

	reason := observed.Reason
	oldState := cluster.Status.State
	cluster.Status.State = observed.Ray.State
	switch observed.Ray.State {
	case v2pb.RAY_CLUSTER_STATE_READY:
		// Becoming ready is what flips Launched to ClusterReady; a cluster that is
		// already there keeps the condition, and its timestamp, as it is.
		if oldState != v2pb.RAY_CLUSTER_STATE_READY || conditionStatus(cluster, LaunchedCondition) != apipb.CONDITION_STATUS_TRUE {
			if setCondition(cluster, LaunchedCondition, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Generation: cluster.Generation,
				Reason:     "ClusterReady",
			}) {
				changed = true
			}
		}
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
				changed = true
			}
		}
	case v2pb.RAY_CLUSTER_STATE_FAILED:
		if setCondition(cluster, SucceededCondition, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: cluster.Generation,
			Reason:     reasonOr(reason, "ClusterFailed"),
		}) {
			changed = true
		}
	case v2pb.RAY_CLUSTER_STATE_UNHEALTHY:
		if setCondition(cluster, SucceededCondition, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Generation: cluster.Generation,
			Reason:     reasonOr(reason, "ClusterUnhealthy"),
		}) {
			changed = true
		}
	case v2pb.RAY_CLUSTER_STATE_UNKNOWN:
		// If the cluster is in an unknown state but its pods report terminal
		// errors, treat it as failed to trigger termination. Only current
		// observations count: this observation's condition errors (the recorded
		// ones were just replaced with them), and the pod-derived entries, which
		// podErrorEventHandler drops once their pod recovers.
		if jobsutils.HasTerminalPodErrors(observed.Ray.PodErrors) || jobsutils.HasTerminalPodErrors(cluster.Status.PodErrors) {
			cluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			if setCondition(cluster, SucceededCondition, jobsutils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Generation: cluster.Generation,
				Reason:     reasonOr(reason, "ClusterFailedWithPodErrors"),
			}) {
				changed = true
			}
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
		if setCondition(cluster, QueuedCondition, jobsutils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_TRUE,
			Generation: cluster.Generation,
			Reason:     "AwaitingAdmission",
			Message:    reason,
		}) {
			changed = true
		}
	}
	return changed || cluster.Status.State != oldState
}

// setCondition sets the condType condition on cluster unless it already says
// the same thing, and reports whether it changed it. Leaving an equal condition
// alone keeps its timestamp, which is what makes re-applying an observation a
// no-op.
func setCondition(cluster *v2pb.RayCluster, condType string, p jobsutils.ConditionUpdateParams) bool {
	for _, cond := range cluster.Status.StatusConditions {
		if cond.GetType() == condType && cond.GetStatus() == p.Status && cond.GetReason() == p.Reason &&
			cond.GetMessage() == p.Message && cond.GetObservedGeneration() == p.Generation {
			return false
		}
	}
	jobsutils.UpdateCondition(jobsutils.GetCondition(&cluster.Status.StatusConditions, condType, cluster.Generation), p)
	return true
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
// also keeps the mutation idempotent under the status write, which re-runs
// its closure on conflict. The list is bounded by two errors per expected pod;
// at the bound, existing entries can still be refreshed but new ones are
// dropped. It reports whether the list changed.
func mergePodErrors(cluster *v2pb.RayCluster, podErrors []*v2pb.PodErrors) bool {
	if len(podErrors) == 0 {
		return false
	}
	changed := false
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
			if *cluster.Status.PodErrors[i] != *podError {
				cluster.Status.PodErrors[i] = podError
				changed = true
			}
			continue
		}
		if len(cluster.Status.PodErrors) >= maxPodErrorLength {
			return changed
		}
		indexByName[podError.GetName()] = len(cluster.Status.PodErrors)
		cluster.Status.PodErrors = append(cluster.Status.PodErrors, podError)
		changed = true
	}
	return changed
}

// conditionPodErrorNames are the names the mapper gives the pod errors it
// reads out of KubeRay's conditions: the condition type, for the two types
// k8sengine's isFailureCondition reports. Pod-derived entries are named after
// their pod, and pod names are lower case, so the two never collide.
var conditionPodErrorNames = map[string]bool{
	string(rayv1.HeadPodReady):             true,
	string(rayv1.RayClusterReplicaFailure): true,
}

// replaceConditionPodErrors makes the condition-derived entries on cluster
// exactly fresh, leaves the pod-derived ones alone, and reports whether that
// changed the list.
func replaceConditionPodErrors(cluster *v2pb.RayCluster, fresh []*v2pb.PodErrors) bool {
	current := make(map[string]bool, len(fresh))
	for _, podError := range fresh {
		current[podError.GetName()] = true
	}
	removed := false
	kept := make([]*v2pb.PodErrors, 0, len(cluster.Status.PodErrors))
	for _, podError := range cluster.Status.PodErrors {
		if conditionPodErrorNames[podError.GetName()] && !current[podError.GetName()] {
			removed = true
			continue
		}
		kept = append(kept, podError)
	}
	if removed {
		cluster.Status.PodErrors = kept
	}
	return mergePodErrors(cluster, fresh) || removed
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
