package raycluster

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	e "github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
)

const (
	_requestTimeout      = 60
	_requeueAfterSeconds = 10
)

// Reconciler reconciles a Ray CRD object
type Reconciler struct {
	client.Client

	env     e.Context

	rayV1Client      *rayv1.RayV1Client
}

const _controllerName = "raycluster"
const _apiVersion = "ray.io/v1"


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RayCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info(fmt.Sprintf("Reconciling ray cluster %s", req.NamespacedName))

	// retrieve the ray cluster
	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.NamespacedName, &rayCluster); err != nil {
		// Resource not found (resource deleted)
		return ctrl.Result{}, nil
	}

	originalRayCluster := rayCluster.DeepCopy()

	result, err := r.reconcile(ctx, logger, &rayCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile %w", err)
	}
	if !reflect.DeepEqual(originalRayCluster, rayCluster) {
		err = r.Status().Update(ctx, &rayCluster)
		if err != nil {
			logger.Error(err, "failed to update status")
			return result, nil
		}
	}

	logger.Info(fmt.Sprintf("Reconcile finished, re-queue after %v", result.RequeueAfter))

	return result, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}).
		Complete(r)
}

// reconcile launches the cluster if it is not already launched
func (r *Reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	rayCluster *v2pb.RayCluster,
) (ctrl.Result, error) {
	shouldBeTerminated := rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID
	status, reason, err := r.getClusterStatus(log, rayCluster)

	res := ctrl.Result{}
	// Here we check the state, take an action and then transition to next state.
	// Only terminal state TERMINATED, FAILED remove request from queue by returning empty ctrl.Result{}
	// For non-terminal state, we should always requeue because we don't have a watch on the dashboard state.
	if reason != nil {
		podError := &v2pb.PodErrors{
			ContainerName: rayCluster.Status.HeadNode.Name,
			ExitCode:      0,
			Reason:        *reason,
		}
		rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
	}
	if err != nil {
		if IsNotFoundError(err) && !shouldBeTerminated {
			log.Info(rayCluster.Status.State.String())
			err = r.createCluster(log, rayCluster)
			if err != nil {
				log.Error(err, "failed to create cluster")
				res.RequeueAfter = time.Second * 20
				rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
			}
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
		} else if IsNotFoundError(err) && shouldBeTerminated {
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
		} else {
			res.RequeueAfter = time.Second * 20
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		}
	} else if status != nil {
		if shouldBeTerminated {
			err := r.deleteClusterStatus(log, rayCluster)
			if err != nil {
				res.RequeueAfter = time.Second * 20
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
			log.Info("cluster is ready, we continue to re-queue until receiving termination signal")
			res.RequeueAfter = time.Second * 20
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_READY
		} else {
			res.RequeueAfter = time.Second * 20
			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
		}
	} else {
		res.RequeueAfter = time.Second * 20
	}
	return res, nil
	//
	//case v2pb.RAY_CLUSTER_STATE_PROVISIONING:
	//	log.Info(rayCluster.Status.State.String())
	//	if rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
	//		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
	//		return ctrl.Result{}, nil
	//	}
	//	status, reason, err := r.getClusterStatus(log, rayCluster)
	//	if err != nil {
	//		if IsNotFoundError(err) {
	//			log.Info("Resource not found, marking as terminated.")
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
	//			return ctrl.Result{}, nil
	//		}
	//		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
	//		if reason != nil {
	//			podError := &v2pb.PodErrors{
	//				ContainerName: rayCluster.Status.HeadNode.Name,
	//				ExitCode:      0,
	//				Reason:        *reason,
	//			}
	//			rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
	//		}
	//		break
	//	}
	//	if reason != nil {
	//		podError := &v2pb.PodErrors{
	//			ContainerName: rayCluster.Status.HeadNode.Name,
	//			ExitCode:      0,
	//			Reason:        *reason,
	//		}
	//		rayCluster.Status.PodErrors = append(rayCluster.Status.PodErrors, podError)
	//	}
	//	if status != nil && r.isTerminatedState(*status) {
	//		if *status == "failed" {
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
	//		} else if *status == "terminated" {
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
	//		} else if *status == "unknown" {
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_UNKNOWN
	//		}
	//		return ctrl.Result{}, nil
	//	} else if *status == "ready" {
	//		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_READY
	//	}
	//	break
	//case v2pb.RAY_CLUSTER_STATE_TERMINATING:
	//	log.Info(rayCluster.Status.State.String())
	//	err := r.deleteClusterStatus(log, rayCluster)
	//	if err != nil {
	//		if IsNotFoundError(err) {
	//			log.Info("Resource not found, marking as terminated.")
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATED
	//		} else {
	//			rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_FAILED
	//		}
	//		break
	//	}
	//	// we check it back to provioning for checking the status
	//	rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_PROVISIONING
	//case v2pb.RAY_CLUSTER_STATE_READY:
	//	log.Info("cluster is ready, we do nothing but continue requeue until the job finishes and received termination signal")
	//	if  rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
	//		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
	//		return ctrl.Result{}, nil
	//	}
	//case v2pb.RAY_CLUSTER_STATE_TERMINATED:
	//	return ctrl.Result{}, nil
	//default:
	//	log.Info(rayCluster.Status.State.String())
	//	if rayCluster.Spec.Termination != nil && rayCluster.Spec.Termination.Type != v2pb.TERMINATION_TYPE_INVALID {
	//		rayCluster.Status.State = v2pb.RAY_CLUSTER_STATE_TERMINATING
	//		return ctrl.Result{}, nil
	//	}
	//	return ctrl.Result{}, nil
	//}
	//
	//return ctrl.Result{RequeueAfter: time.Second * _requeueAfterSeconds}, nil
}

func (r *Reconciler) createCluster(log logr.Logger, cluster *v2pb.RayCluster) error {
	rayV1Cluster := &v1.RayCluster{
		TypeMeta:   metav1.TypeMeta{
			Kind:       "RayCluster",
			APIVersion: _apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:               cluster.Name,
			Namespace:                  cluster.Namespace,
		},
		Spec:       v1.RayClusterSpec{
			EnableInTreeAutoscaling: nil,
			HeadGroupSpec:           v1.HeadGroupSpec{
				ServiceType:    corev1.ServiceType(cluster.Spec.Head.ServiceType),
				RayStartParams: cluster.Spec.Head.RayStartParams,
				Template:       corev1.PodTemplateSpec{
					Spec:       corev1.PodSpec{
						Containers:                    []corev1.Container{
							convertPodSpecToContainer(cluster.Spec.Head.Pod),
						},
					},
				},
			},
			RayVersion:       cluster.Spec.RayVersion,
			WorkerGroupSpecs: convertWorkerGroupSpecsToWorkerSpec(cluster.Name, cluster.Spec.Workers),
		},
	}
	createdRayCluster, err := r.rayV1Client.RayClusters(cluster.Namespace).Create(context.TODO(), rayV1Cluster, metav1.CreateOptions{})
	log.Info(fmt.Sprintf("ray cluster %s/%s created", createdRayCluster.Namespace, createdRayCluster.Name))
	if err != nil {
		log.Error(err, "Failed to submit RayCluster")
		return err
	}
	cluster.Status.HeadNode = &v2pb.RayHeadNodeInfo{
		Name: createdRayCluster.Name,
	}
	return nil
}

func (r *Reconciler) getClusterStatus(log logr.Logger, cluster *v2pb.RayCluster) (*v1.ClusterState, *string, error) {
	rayV1Cluster, err := r.rayV1Client.RayClusters(cluster.Namespace).Get(context.TODO(), cluster.Status.HeadNode.Name, metav1.GetOptions{})
	// Fetch the status of the RayCluster
	if err != nil {
		log.Error(err, "Failed to get RayCluster status: %v")
		return nil, nil, err
	}

	return &rayV1Cluster.Status.State, &rayV1Cluster.Status.Reason, nil
}

func (r *Reconciler) deleteClusterStatus(log logr.Logger, cluster *v2pb.RayCluster) error {
	err := r.rayV1Client.RayClusters(cluster.Namespace).Delete(context.TODO(), cluster.Name, metav1.DeleteOptions{})
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
		Limits:   corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%d", resourceSpec.Cpu)),
			corev1.ResourceMemory: resource.MustParse(resourceSpec.Memory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%d", resourceSpec.Cpu)),
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
			Name:      env.Name,
			Value:     env.Value,
		}
		envVars = append(envVars, newEnv)
	}
	return envVars
}

func convertPodSpecToContainer(pod *v2pb.PodSpec) corev1.Container {
	return corev1.Container{
		Name:                     pod.Name,
		Image:                    pod.Image,
		//ImagePullPolicy: "Never",
		Command:                  pod.Command,
		EnvFrom:                  []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "michelangelo-config",
					},
				},
			},
		},
		Env:                      convertEnvVar(pod.Env),
		Resources:                convertResource(pod.Resource),
		//VolumeMounts:             []corev1.VolumeMount{
		//	{
		//		Name:              "log-volume",
		//		MountPath:         "/tmp/ray",
		//	},
		//},
	}
}

// Function to convert WorkerGroupSpecs to JSON
func convertWorkerGroupSpecsToWorkerSpec(clusterName string, workers []*v2pb.RayWorkerSpec) []v1.WorkerGroupSpec {
	workerGroupSpecsJson := make([]v1.WorkerGroupSpec, len(workers))
	for i, workerGroup := range workers {
		workerGroupMap := v1.WorkerGroupSpec{
			GroupName: fmt.Sprintf("wg-%v", clusterName),
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

// IsNotFoundError checks if the error is not found error
func IsNotFoundError(err error) bool {
	if strings.Contains(err.Error(), "not found") {
		return true
	} else if e, ok := status.FromError(err); ok {
		return e.Code() == codes.NotFound
	}
	return false
}
