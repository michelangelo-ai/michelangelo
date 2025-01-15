package raycluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/apiutil"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
)

const (
	requeueAfter = time.Second * 10
)

// Reconciler reconciles a Ray Cluster object
type Reconciler struct {
	client.Client
	env env.Context
	rayV1Client *rayv1.RayV1Client
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ray cluster ", "namespacedName", req.NamespacedName)

	res := ctrl.Result{}
	// retrieve the ray cluster
	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.NamespacedName, &rayCluster); err != nil {
		// Resource not found (resource deleted)
		if apiutil.IsNotFoundError(err) {
			_, _, err = r.getClusterStatus(ctx, logger, req.Namespace, req.Name)
			if err != nil {
				if apiutil.IsNotFoundError(err) {
					// cluster is deleted or terminated, exit
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}
			err = r.deleteCluster(ctx, logger, req.Namespace, req.Name)
			if err != nil {
				res.RequeueAfter = requeueAfter
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			} else {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			}
			return ctrl.Result{}, nil
		}
		res.RequeueAfter = requeueAfter
		return ctrl.Result{}, err
	}

	originalRayCluster := rayCluster.DeepCopy()

	shouldBeTerminated := rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID
	status, reason, err := r.getClusterStatus(ctx, logger, rayCluster.Namespace, rayCluster.Name)


	if reason != nil {
		podError := &v2pb.PodErrors{
			ContainerName: rayCluster.Name,
			ExitCode:      0,
			Reason:        *reason,
		}
		rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
	}
	if err != nil {
		logger.Error(err, "error for getting ray cluster")
		if apiutil.IsNotFoundError(err) && !shouldBeTerminated {
			logger.Info("creating new ray cluster")
			err = r.createCluster(ctx, logger, &rayCluster)
			if err != nil {
				logger.Error(err, "failed to create cluster")
				res.RequeueAfter = requeueAfter
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			}
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
		} else if apiutil.IsNotFoundError(err) && shouldBeTerminated {
			logger.Info("cluster is terminated")
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
		} else {
			res.RequeueAfter = requeueAfter
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		}
	} else if status != nil {
		logger.Info("get ray cluster with status ", "status", *status)
		if shouldBeTerminated {
			err = r.deleteCluster(ctx, logger, rayCluster.Namespace, rayCluster.Name)
			if err != nil {
				res.RequeueAfter = requeueAfter
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			} else {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
			}
		} else if r.isTerminatedState(*status) {
			if *status == "failed" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			} else if *status == "terminated" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
			} else if *status == "unknown" {
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_UNKNOWN
			}
		} else if *status == "ready" {
			logger.Info("cluster is ready, we continue to re-queue until receiving termination signal")
			res.RequeueAfter = requeueAfter
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_READY
		} else {
			res.RequeueAfter = requeueAfter
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		}
	} else {
		res.RequeueAfter = requeueAfter
	}

	if !reflect.DeepEqual(originalRayCluster, rayCluster) {
		err = r.Status().Update(ctx, &rayCluster)
		if err != nil {
			logger.Error(err, "failed to update status")
			return res, nil
		}
	}

	logger.Info("Reconcile finished, re-queue after", "requeueAfter",  res.RequeueAfter)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}).
		Complete(r)
}

func (r *Reconciler) createCluster(ctx context.Context, log logr.Logger, cluster *v2pb.RayCluster) error {
	rayV1Cluster := &v1.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Spec: v1.RayClusterSpec{
			EnableInTreeAutoscaling: nil,
			HeadGroupSpec: v1.HeadGroupSpec{
				ServiceType:    corev1.ServiceType(cluster.Spec.Head.ServiceType),
				RayStartParams: cluster.Spec.Head.RayStartParams,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							convertPodSpecToContainer(cluster.Spec.Head.Pod),
						},
					},
				},
			},
			RayVersion:       cluster.Spec.RayVersion,
			WorkerGroupSpecs: convertWorkerGroupSpecsToWorkerSpec(cluster.Name, cluster.Spec.Workers),
		},
	}
	createdRayCluster, err := r.rayV1Client.RayClusters(cluster.Namespace).Create(ctx, rayV1Cluster, metav1.CreateOptions{})
	log.Info("ray cluster created", "namespace", createdRayCluster.Namespace, "name", createdRayCluster.Name)
	if err != nil {
		log.Error(err, "Failed to submit RayCluster")
		return err
	}
	cluster.Status.HeadNode = &v2pb.RayHeadNodeInfo{
		Name: createdRayCluster.Name,
	}
	return nil
}

func (r *Reconciler) getClusterStatus(ctx context.Context, log logr.Logger, namespace string, name string) (*v1.ClusterState, *string, error) {
	rayV1Cluster, err := r.rayV1Client.RayClusters(namespace).Get(ctx, name, metav1.GetOptions{})
	// Fetch the status of the RayCluster
	if err != nil {
		log.Error(err, "Failed to get RayCluster status: %v")
		return nil, nil, err
	}

	return &rayV1Cluster.Status.State, &rayV1Cluster.Status.Reason, nil
}

func (r *Reconciler) deleteCluster(ctx context.Context, log logr.Logger, namespace string, name string) error {
	err := r.rayV1Client.RayClusters(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "Failed to delete RayCluster: %v")
		return err
	}
	return nil
}

func (r *Reconciler) isTerminatedState(status v1.ClusterState) bool {
	for _, state := range []v1.ClusterState{v1.Failed, v1.Suspended} {
		if status == state {
			return true
		}
	}
	return false
}

func convertResource(resourceSpec *v2pb.ResourceSpec) corev1.ResourceRequirements {
	requestedResource := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", resourceSpec.Cpu)),
			corev1.ResourceMemory: resource.MustParse(resourceSpec.Memory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", resourceSpec.Cpu)),
			corev1.ResourceMemory: resource.MustParse(resourceSpec.Memory),
		},
	}

	if resourceSpec.Gpu > 0 {
		requestedResource.Requests["gpu"] = resource.MustParse(fmt.Sprintf("%d", resourceSpec.Gpu))
	}
	if resourceSpec.GpuSku != "" {
		requestedResource.Requests["gpu_sku"] = resource.MustParse(resourceSpec.GpuSku)
	}
	if resourceSpec.FileDescriptors != 0 {
		requestedResource.Requests["file_descriptors"] = resource.MustParse(fmt.Sprintf("%d", resourceSpec.FileDescriptors))
	}
	if resourceSpec.DiskSize != "" && resourceSpec.DiskSize != "0" {
		requestedResource.Requests["disk_size"] = resource.MustParse(resourceSpec.DiskSize)
	}
	return requestedResource
}

func convertEnvVar(environments []*v2pb.Environment) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0)
	for _, env := range environments {
		newEnv := corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		}
		envVars = append(envVars, newEnv)
	}
	return envVars
}

func convertPodSpecToContainer(pod *v2pb.PodSpec) corev1.Container {
	return corev1.Container{
		Name:  pod.Name,
		Image: pod.Image,
		Command: pod.Command,
		EnvFrom: []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "michelangelo-config",
					},
				},
			},
		},
		Env:       convertEnvVar(pod.Env),
		Resources: convertResource(pod.Resource),
	}
}

// Function to convert WorkerGroupSpecs to JSON
func convertWorkerGroupSpecsToWorkerSpec(clusterName string, workers []*v2pb.RayWorkerSpec) []v1.WorkerGroupSpec {
	workerGroupSpecsJson := make([]v1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		workerGroupMap := v1.WorkerGroupSpec{
			GroupName:      fmt.Sprintf("wg-%v", clusterName),
			Replicas:       &workerGroup.MinInstances,
			MinReplicas:    &workerGroup.MinInstances,
			MaxReplicas:    &workerGroup.MaxInstances,
			RayStartParams: workerGroup.RayStartParams,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{convertPodSpecToContainer(workerGroup.Pod)},
				},
			},
		}
		workerGroupSpecsJson[i] = workerGroupMap
	}
	return workerGroupSpecsJson
}


