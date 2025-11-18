package cluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	// Defines the delay before retrying the reconciliation process
	requeueAfter = time.Second * 10
)

// Reconciler handles the lifecycle of Ray Cluster objects in the Kubernetes cluster
type Reconciler struct {
	client.Client                    // Kubernetes API client for managing resources
	rayv1.RayV1Interface             // Ray-specific Kubernetes client
	env                  env.Context // Environment context for configuration
}

// Reconcile ensures the desired state of the Ray Cluster matches the actual state in the cluster.
// It implements the Kubernetes reconciliation loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Initialize logger from the context for scoped logging
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ray cluster ", "namespacedName", req.NamespacedName)

	// Initialize the result object to define next actions
	res := ctrl.Result{}

	// Retrieve the RayCluster custom resource using the request's namespace and name
	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.NamespacedName, &rayCluster); err != nil {
		// If the resource is not found, assume it has been deleted
		if utils.IsNotFoundError(err) {
			// Check the status of the cluster to confirm deletion
			_, _, err = r.getClusterStatus(ctx, logger, req.Namespace, req.Name)
			if err != nil {
				// Return if the cluster is already deleted
				if utils.IsNotFoundError(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			// Attempt to delete the cluster resources
			err = r.deleteCluster(ctx, logger, req.Namespace, req.Name)
			if err != nil {
				// Requeue if deletion fails
				res.RequeueAfter = requeueAfter
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			} else {
				// Mark the cluster as terminated
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
			}
			return ctrl.Result{}, nil
		}
		// Requeue for errors other than not found
		res.RequeueAfter = requeueAfter
		return ctrl.Result{}, err
	}

	// Create a copy of the original RayCluster for comparison
	originalRayCluster := rayCluster.DeepCopy()

	// Check if the cluster should be terminated based on its spec
	shouldBeTerminated := rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID

	// Get the current status of the RayCluster
	status, reason, err := r.getClusterStatus(ctx, logger, rayCluster.Namespace, rayCluster.Name)

	// Handle errors and update the cluster state accordingly
	if reason != nil && *reason != "" {
		podError := &v2pb.PodErrors{
			ContainerName: rayCluster.Name,
			ExitCode:      0,
			Reason:        *reason,
		}
		rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
	}
	if err != nil {
		// Handle cluster status retrieval errors
		if utils.IsNotFoundError(err) && !shouldBeTerminated {
			logger.Info("creating new ray cluster")
			err = r.createCluster(ctx, logger, &rayCluster)
			if err != nil {
				logger.Error(err, "failed to create ray cluster",
					"operation", "create_cluster",
					"namespace", req.Namespace,
					"name", req.Name)
				res.RequeueAfter = requeueAfter
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
				return res, fmt.Errorf("create ray cluster %q: %w", req.NamespacedName, err)
			}
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
		} else if utils.IsNotFoundError(err) && shouldBeTerminated {
			logger.Info("cluster is terminated")
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
		} else {
			res.RequeueAfter = requeueAfter
		}
	} else if status != nil {
		// Handle cases where cluster status is retrieved successfully
		logger.Info("get ray cluster with status ", "status", status.State)
		if shouldBeTerminated {
			// Delete the cluster if termination is requested
			logger.Info("terminating cluster")
			err = r.deleteCluster(ctx, logger, rayCluster.Namespace, rayCluster.Name)
			if err != nil {
				res.RequeueAfter = requeueAfter
			} else {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			}
		} else {
			terminateStateMap := map[v1.ClusterState]v2pb.RayClusterState{
				v1.Failed: v2pb.RAY_CLUSTER_STATE_FAILED,
				v1.Ready:  v2pb.RAY_CLUSTER_STATE_READY,
			}
			if newState, exists := terminateStateMap[status.State]; exists {
				rayCluster.Status.State = newState
				if newState == v2pb.RAY_CLUSTER_STATE_READY {
					logger.Info("Cluster is ready, re-queuing until receiving termination signal")
					res.RequeueAfter = requeueAfter
				}
			} else {
				res.RequeueAfter = requeueAfter
			}
		}
	} else {
		// Requeue in cases where no status is retrieved
		res.RequeueAfter = requeueAfter
	}

	// Update the RayCluster status if any changes occurred
	if !reflect.DeepEqual(originalRayCluster, rayCluster) {
		err = r.Status().Update(ctx, &rayCluster)
		if err != nil {
			logger.Error(err, "failed to update ray cluster status",
				"operation", "update_status",
				"namespace", req.Namespace,
				"name", req.Name)
			return res, fmt.Errorf("update ray cluster status for %q: %w", req.NamespacedName, err)
		}
	}

	// Log the completion of the reconciliation process
	logger.Info("Reconcile finished, re-queue after", "requeueAfter", res.RequeueAfter)

	return res, nil
}

// Register adds the Reconciler to the controller manager
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}). // Watch for changes in RayCluster custom resources
		Complete(r)
}

// createCluster initializes a new RayCluster resource and submits it to the Kubernetes cluster
func (r *Reconciler) createCluster(ctx context.Context, log logr.Logger, cluster *v2pb.RayCluster) error {
	// Define the RayCluster spec based on the input cluster
	rayV1Cluster := &v1.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: v1.RayClusterSpec{
			HeadGroupSpec: v1.HeadGroupSpec{
				ServiceType:    corev1.ServiceType(cluster.Spec.Head.ServiceType),
				RayStartParams: cluster.Spec.Head.RayStartParams,
				Template:       *cluster.Spec.Head.Pod,
			},
			RayVersion:       cluster.Spec.RayVersion,
			WorkerGroupSpecs: convertWorkerGroupSpecsToWorkerSpec(cluster.Name, cluster.Spec.Workers),
		},
	}
	// Create the RayCluster resource in the Kubernetes cluster
	createdRayCluster, err := r.RayClusters(cluster.Namespace).Create(ctx, rayV1Cluster, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "failed to create ray cluster",
			"operation", "create_cluster",
			"namespace", cluster.Namespace,
			"name", cluster.Name)
		return fmt.Errorf("create ray cluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}
	log.Info("ray cluster created", "namespace", createdRayCluster.Namespace, "name", createdRayCluster.Name)
	// Update the cluster's head node information
	cluster.Status.HeadNode = &v2pb.RayHeadNodeInfo{
		// TODO(#553): use createdRayCluster.Status.Head.PodName after upgrading to a newer version
		Name: cluster.Spec.Head.Pod.Name,
		Ip:   createdRayCluster.Status.Head.PodIP,
	}
	return nil
}

// getClusterStatus retrieves the current status of a RayCluster resource
func (r *Reconciler) getClusterStatus(ctx context.Context, log logr.Logger, namespace string, name string) (*v1.RayClusterStatus, *string, error) {
	// Fetch the RayCluster resource by name and namespace
	rayV1Cluster, err := r.RayClusters(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get ray cluster status",
			"operation", "get_status",
			"namespace", namespace,
			"name", name)
		return nil, nil, fmt.Errorf("get ray cluster %s/%s status: %w", namespace, name, err)
	}
	// Check for empty resource to handle errors gracefully
	if rayV1Cluster != nil && rayV1Cluster.Name == "" {
		return nil, nil, apiErrors.NewNotFound(v1.Resource("rayclusters"), name)
	}
	return &rayV1Cluster.Status, &rayV1Cluster.Status.Reason, nil
}

// deleteCluster removes the specified RayCluster resource from the Kubernetes cluster
func (r *Reconciler) deleteCluster(ctx context.Context, log logr.Logger, namespace string, name string) error {
	// Delete the RayCluster resource and handle errors
	err := r.RayClusters(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "failed to delete ray cluster",
			"operation", "delete_cluster",
			"namespace", namespace,
			"name", name)
		return fmt.Errorf("delete ray cluster %s/%s: %w", namespace, name, err)
	}
	return nil
}

// convertWorkerGroupSpecsToWorkerSpec converts worker specifications into a format suitable for the RayCluster resource
func convertWorkerGroupSpecsToWorkerSpec(clusterName string, workers []*v2pb.RayWorkerSpec) []v1.WorkerGroupSpec {
	workerGroupSpecsJson := make([]v1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		workerGroupMap := v1.WorkerGroupSpec{
			GroupName:      fmt.Sprintf("wg-%v", clusterName),
			Replicas:       &workerGroup.MinInstances,
			MinReplicas:    &workerGroup.MinInstances,
			MaxReplicas:    &workerGroup.MaxInstances,
			RayStartParams: workerGroup.RayStartParams,
			Template:       *workerGroup.Pod,
		}
		workerGroupSpecsJson[i] = workerGroupMap
	}
	return workerGroupSpecsJson
}
