package rayjob

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	e "github.com/michelangelo-ai/michelangelo/go/base/env"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	rayv1 "github.com/ray-project/kuberay/ray-operator/pkg/client/clientset/versioned/typed/ray/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	_requeueAfterSeconds = 20
)

// Reconciler reconciles a Ray Job object
type Reconciler struct {
	client.Client

	env e.Context

	rayV1Client *rayv1.RayV1Client
}

const _controllerName = "rayv2"
const _apiVersion = "ray.io/v1"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info(fmt.Sprintf("Reconciling ray job %s", req.NamespacedName))

	// retrieve the ray job
	var rayJob v2pb.RayJob
	if err := r.Get(ctx, req.NamespacedName, &rayJob); err != nil {
		// Resource not found (resource deleted)
		return ctrl.Result{}, nil
	}

	originalRayJob := rayJob.DeepCopy()

	result, err := r.reconcile(ctx, logger, &rayJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to reconcile %w", err)
	}
	if !reflect.DeepEqual(originalRayJob, rayJob) {
		err = r.Status().Update(ctx, &rayJob)
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
		For(&v2pb.RayJob{}).
		Complete(r)
}

func (r *Reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	rayJob *v2pb.RayJob,
) (ctrl.Result, error) {
	res := ctrl.Result{}

	if rayJob.Spec.Cluster == nil {
		rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
		rayJob.Status.Message = "cluster is not set"
		return res, nil
	}

	rayCluster := &v2pb.RayCluster{}

	err := r.Get(ctx, types.NamespacedName{
		Namespace: rayJob.Spec.Cluster.Namespace,
		Name:      rayJob.Spec.Cluster.Name,
	}, rayCluster)
	if err != nil {
		log.Error(err, "error to get cluster")
		rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
		rayJob.Status.Message = "cluster is not found"
		return res, err
	}

	status, jobFailedReason, err := r.getJobStatus(log, rayJob)
	if err != nil {
		log.Error(err, "error to get ray job")
		err := r.createJob(log, rayJob, rayCluster)
		if err != nil {
			log.Error(err, "failed to create the ray job in ray operator")
			rayJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
			rayJob.Status.Message = fmt.Sprintf("failed to create the ray job in cluster %s/%s", rayCluster.Namespace, rayCluster.Name)
			return res, nil
		}
		rayJob.Status.State = v2pb.RAY_JOB_STATE_INITIALIZING
		res.RequeueAfter = _requeueAfterSeconds
	} else if status != nil {
		if r.isTerminatedState(*status) {
			log.Info(fmt.Sprintf("job finished with status %s", *status))
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
			log.Info("job is running")
			rayJob.Status.State = v2pb.RAY_JOB_STATE_RUNNING
			res.RequeueAfter = _requeueAfterSeconds
		}
	} else {
		log.Info("unknown status, re-queuing")
		res.RequeueAfter = _requeueAfterSeconds
	}

	return res, nil
}

func (r *Reconciler) createJob(log logr.Logger, job *v2pb.RayJob, cluster *v2pb.RayCluster) error {
	rayJob := &v1.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RayJob",
			APIVersion: _apiVersion,
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

	createdRayJob, err := r.rayV1Client.RayJobs(cluster.Namespace).Create(context.TODO(), rayJob, metav1.CreateOptions{})
	log.Info(fmt.Sprintf("ray job %s/%s created", createdRayJob.Namespace, createdRayJob.Name))
	if err != nil {
		log.Error(err, "Failed to submit RayJob")
		return err
	}

	return nil
}

func (r *Reconciler) getJobStatus(log logr.Logger, rayJob *v2pb.RayJob) (*v1.JobStatus, *v1.JobFailedReason, error) {
	rayV1Job, err := r.rayV1Client.RayJobs(rayJob.Namespace).Get(context.TODO(), rayJob.Name, metav1.GetOptions{})
	// Fetch the status of the RayJob
	if err != nil {
		log.Error(err, "Failed to get RayJob status: %v")
		return nil, nil, err
	}

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
