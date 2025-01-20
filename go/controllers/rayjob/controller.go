package rayjob

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	requeueAfter = time.Second * 10
	apiVersion = "ray.io/v1"
)

// Reconciler reconciles a Ray Job object
type Reconciler struct {
	client.Client
	rayv1.RayV1Interface
	env         env.Context
}


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	print(fmt.Sprintf("reconciling %s " ,req.NamespacedName))
	logger.Info("Reconciling ray job", "namespacedName", req.NamespacedName)
	res := ctrl.Result{}

	// retrieve the ray job
	var rayJob v2pb.RayJob
	if err := r.Get(ctx, req.NamespacedName, &rayJob); err != nil {
		print("failed to get ray job, it could be deleted")
		// Resource not found (resource deleted)
		if utils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}
	print("got ray job")
	// original copy of ray job to determine if we need to update the status
	originalRayJob := rayJob.DeepCopy()

	if rayJob.Spec.Cluster == nil {
		print("no cluster")
		// when cluster is not provided, exit with failed state
		rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
		rayJob.Status.Message = "cluster is not set"
	} else {
		print("got cluster in spec")
		// get ray cluster entity for further processing
		rayCluster := &v2pb.RayCluster{}

		err := r.Get(ctx, types.NamespacedName{
			Namespace: rayJob.Spec.Cluster.Namespace,
			Name:      rayJob.Spec.Cluster.Name,
		}, rayCluster)
		if err != nil {
			print("failed to get cluster \n")
			print(err.Error())
			if utils.IsNotFoundError(err) {
				rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
				rayJob.Status.Message = fmt.Sprintf("failed to find cluster %s/%s", rayCluster.Namespace, rayCluster.Name)
			} else {
				// failed to fetch cluster entity, requeue
				logger.Error(err, "error to get cluster, requeue")
				res.RequeueAfter = requeueAfter
			}
		} else {
			print("get cluster entity")
			if rayCluster.Status.State != v2pb.RAY_CLUSTER_STATE_READY {
				// If cluster is not in ready state, we wait until it's ready
				logger.Info("cluster is not ready, waiting")
				rayJob.Status.Message = fmt.Sprintf("cluster %s/%s is not ready", rayCluster.Namespace, rayCluster.Name)
				res.RequeueAfter = requeueAfter
			} else {
				print("checking status\n")
				// we start checking to see if the job has created by checking job status
				status, jobFailedReason, jobErr := r.getJobStatus(ctx, logger, &rayJob)
				if jobErr != nil {
					logger.Error(jobErr, "error to get ray job")
					err = r.createJob(ctx, logger, &rayJob, rayCluster)
					if err != nil {
						logger.Error(err, "failed to create the ray job in ray operator")
						rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
						rayJob.Status.Message = fmt.Sprintf("failed to create the ray job in cluster %s/%s", rayCluster.Namespace, rayCluster.Name)
					}
					rayJob.Status.State = v2pb.RAY_JOB_STATE_INITIALIZING
					res.RequeueAfter = requeueAfter
				} else if status != nil {
					// if the job has created, we keep checking the status to see if it reaches the final state
					if r.isTerminatedState(*status) {
						logger.Info("job finished with status", "status", *status)
						rayJob.Status.JobStatus = string(*status)
						if *status == "SUCCEEDED" {
							rayJob.Status.State = v2pb.RAY_JOB_STATE_SUCCEEDED
						} else if *status == "FAILED" {
							rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
						} else if *status == "STOPPED" {
							rayJob.Status.State = v2pb.RAY_JOB_STATE_KILLED
						}
						if jobFailedReason != nil {
							rayJob.Status.Message = string(*jobFailedReason)
						}
					} else {
						// job is still running, wait
						logger.Info("job is running")
						rayJob.Status.State = v2pb.RAY_JOB_STATE_RUNNING
						res.RequeueAfter = requeueAfter
					}
				} else {
					// invalid status, we requeue
					logger.Info("unknown status, re-queuing")
					res.RequeueAfter = requeueAfter
				}
			}
		}
	}

	if !reflect.DeepEqual(originalRayJob, rayJob) {
		print(fmt.Sprintf("------updating object status to %s\n", rayJob.Status.State))
		// update the resource in ETCD
		if r.isRayJobTerminatedState(rayJob.Status.State) {
			utils.MarkImmutable(&rayJob)
		}
		err := r.Status().Update(ctx, &rayJob)
		if err != nil {
			print("failed to update object")
			print(err.Error())
			logger.Error(err, "failed to update status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
		print("updated object")
	}

	logger.Info("reconcile finished, re-queue after ", "requeueAfter", res.RequeueAfter)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayJob{}).
		Complete(r)
}

func (r *Reconciler) createJob(ctx context.Context, log logr.Logger, job *v2pb.RayJob, cluster *v2pb.RayCluster) error {
	rayJob := &v1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RayJob",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
		},
		Spec: v1.RayJobSpec{
			ClusterSelector: map[string]string{
				"ray.io/cluster":      cluster.Name,
				"rayClusterNamespace": cluster.Namespace,
			},
			Entrypoint: job.Spec.Entrypoint,
		},
	}

	createdRayJob, err := r.RayJobs(cluster.Namespace).Create(ctx, rayJob, metav1.CreateOptions{})
	job.Status.JobId = createdRayJob.Status.JobId
	job.Status.DashboardUrl = createdRayJob.Status.DashboardURL
	job.Status.JobDeploymentStatus = string(createdRayJob.Status.JobDeploymentStatus)
	log.Info("ray job created", "namespace", createdRayJob.Namespace, "name", createdRayJob.Name)
	if err != nil {
		log.Error(err, "Failed to submit RayJob")
		return err
	}

	return nil
}

func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, rayJob *v2pb.RayJob) (*v1.JobStatus, *v1.JobFailedReason, error) {
	rayV1Job, err := r.RayJobs(rayJob.Namespace).Get(ctx, rayJob.Name, metav1.GetOptions{})
	// Fetch the status of the RayJob
	if err != nil {
		print("failed to get ray job status\n")
		print(err.Error())
		logger.Error(err, "failed to get RayJob status: %v")
		return nil, nil, err
	}
	rayJob.Status.JobId = rayV1Job.Status.JobId
	rayJob.Status.DashboardUrl = rayV1Job.Status.DashboardURL
	rayJob.Status.JobDeploymentStatus = string(rayV1Job.Status.JobDeploymentStatus)

	return &rayV1Job.Status.JobStatus, &rayV1Job.Status.Reason, nil
}

func (r *Reconciler) isTerminatedState(status v1.JobStatus) bool {
	for _, state := range []v1.JobStatus{v1.JobStatusSucceeded, v1.JobStatusFailed, v1.JobStatusStopped} {
		if status == state {
			// Return OK. The job submission has reached a terminal status.
			return true
		}
	}
	return false
}

func (r *Reconciler) isRayJobTerminatedState(status v2pb.RayJobState) bool {
	for _, state := range []v2pb.RayJobState{v2pb.RAY_JOB_STATE_FAILED, v2pb.RAY_JOB_STATE_SUCCEEDED, v2pb.RAY_JOB_STATE_KILLED} {
		if status == state {
			// Return OK. The job submission has reached a terminal status.
			return true
		}
	}
	return false
}
