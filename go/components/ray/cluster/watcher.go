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
	// We only care about the head pod for the add event.
	if !jobsutils.IsRayHeadNode(pod) {
		return
	}
	r.podEventHandler(pod)
}

func (r *Reconciler) podUpdateEventHandler(_, newObj interface{}) {
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	// We only care about the head pod for the update event. We re-inspect even
	// if already recorded to handle the case where we missed the original event.
	if !jobsutils.IsRayHeadNode(pod) {
		return
	}
	r.podEventHandler(pod)
}

func (r *Reconciler) podEventHandler(pod *corev1.Pod) {
	log := r.logger.WithValues("pod_name", pod.Name)

	if pod.Status.Phase != corev1.PodRunning {
		log.Info("head pod is not in running state, will wait for next pod event",
			"pod_phase", pod.Status.Phase)
		return
	}

	clusterName, err := getClusterName(pod)
	if err != nil {
		log.Error(err, "unable to get cluster name")
		return
	}
	log = log.WithValues("ray_cluster", clusterName)

	clientPort, err := getClientPort(pod)
	if err != nil {
		log.Error(err, "unable to get head pod client port")
	}

	jupyterNotebookPort, err := getJupyterNotebookPort(pod)
	if err != nil {
		log.Error(err, "unable to get head pod jupyter notebook port")
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

	ctx, cancel := context.WithTimeout(context.Background(), _eventHandlerTimeout)
	defer cancel()

	var rayCluster v2pb.RayCluster
	if err = r.Get(ctx, namespace, clusterName, &metav1.GetOptions{}, &rayCluster); err != nil {
		log.Error(err, "could not fetch the ray cluster for the pod - not processing pod delete event further")
		return
	}

	if pod.Status.Phase == corev1.PodFailed {
		log.Info("pod failed", "reason", pod.Status.Reason, "status_message", pod.Status.Message,
			"container_statuses", pod.Status.ContainerStatuses, "init_container_statuses", pod.Status.InitContainerStatuses)
	}

	// Limit the number of possible pod errors to avoid large status updates.
	maxPodErrorLength := 2 * (1 + jobsutils.NumRayWorkers(&rayCluster))
	if len(rayCluster.Status.PodErrors) >= maxPodErrorLength {
		log.Info("reached max pod errors limit, skipping further pod error updates",
			"max_pod_error_length", maxPodErrorLength,
			"current_pod_error_length", len(rayCluster.Status.PodErrors))
		return
	}

	podError := jobsutils.GetErrorFromPodStatus(pod, func(containerStatus corev1.ContainerStatus) bool {
		return containerStatus.Name == constants.HeadContainerName || containerStatus.Name == constants.WorkerContainerName
	})
	// If no container errors are found we do not need to update the status.
	if podError == nil {
		return
	}

	if err = jobsutils.UpdateStatusWithRetries(ctx, r, &rayCluster,
		func(obj client.Object) {
			cluster := obj.(*v2pb.RayCluster)
			cluster.Status.PodErrors = append(cluster.Status.PodErrors, podError)
		}, &metav1.UpdateOptions{
			FieldManager: "podDeleteEventHandler",
		}); err != nil {
		log.Error(err, "could not update the ray cluster with the pod error")
	}
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
		log.Error(err, "could not fetch the global ray cluster")
		return
	}

	killing := jobsutils.GetCondition(&globalCluster.Status.StatusConditions, KillingCondition, globalCluster.Generation)

	if killing.Status != apipb.CONDITION_STATUS_TRUE {
		// Cluster was deleted externally without going through the controller.
		if err := jobsutils.UpdateStatusWithRetries(ctx, r, &globalCluster,
			func(obj client.Object) {
				cluster := obj.(*v2pb.RayCluster)
				succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
				jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Reason:     constants.ClusterKilled,
					Generation: cluster.Generation,
					Message:    fmt.Sprintf("ray cluster was deleted externally, state: %+v", local.Status.State),
				})
				killedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, KilledCondition, cluster.Generation)
				jobsutils.UpdateCondition(killedCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Generation: cluster.Generation,
				})
			}, &metav1.UpdateOptions{
				FieldManager: "rayClusterDeleteEventHandler",
			}); err != nil {
			log.Error(err, "failed to update status on external delete")
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

	newState := mapKubeRayClusterState(local.Status.State)

	// Cache re-sync gives Update events even when the local cluster did not
	// change. Exit early if global already reflects the local state to avoid
	// unnecessary CRD writes.
	if globalCluster.Status.State == newState && newState == v2pb.RAY_CLUSTER_STATE_READY {
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

			switch newState {
			case v2pb.RAY_CLUSTER_STATE_READY:
				launchedCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, LaunchedCondition, cluster.Generation)
				jobsutils.UpdateCondition(launchedCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_TRUE,
					Generation: cluster.Generation,
					Reason:     "ClusterReady",
				})
			case v2pb.RAY_CLUSTER_STATE_FAILED:
				succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
				jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Generation: cluster.Generation,
					Reason:     "ClusterFailed",
				})
			case v2pb.RAY_CLUSTER_STATE_UNHEALTHY:
				succeededCond := jobsutils.GetCondition(&cluster.Status.StatusConditions, SucceededCondition, cluster.Generation)
				jobsutils.UpdateCondition(succeededCond, jobsutils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Generation: cluster.Generation,
					Reason:     "ClusterUnhealthy",
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
						Reason:     "ClusterFailedWithPodErrors",
					})
				}
			}
		}, &metav1.UpdateOptions{
			FieldManager: "rayClusterEventHandler",
		}); err != nil {
		log.Error(err, "failed to update global ray cluster status")
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
	default:
		return v2pb.RAY_CLUSTER_STATE_UNKNOWN
	}
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
