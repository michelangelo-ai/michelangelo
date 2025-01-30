package sparkjob

import (
	"context"
	"fmt"
	v1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"reflect"
	"time"

	"github.com/go-logr/logr"

	sparkv1beta2 "github.com/kubeflow/spark-operator/pkg/client/clientset/versioned/typed/sparkoperator.k8s.io/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	requeueAfter = time.Second * 10
	apiVersion   = "spark.io/v1"
)

// Reconciler reconciles a Ray Job object
type Reconciler struct {
	client.Client
	env env.Context
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling spark job", "namespacedName", req.NamespacedName)
	res := ctrl.Result{}

	// retrieve the spark job
	var sparkJob v2pb.SparkJob
	if err := r.Get(ctx, req.NamespacedName, &sparkJob); err != nil {
		// Resource not found (resource deleted)
		if utils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}
	// original copy of spark job to determine if we need to update the status
	originalRayJob := sparkJob.DeepCopy()

	// we start checking to see if the job has created by checking job status
	status, jobFailedReason, jobErr := r.getJobStatus(ctx, logger, &sparkJob)
	if jobErr != nil {
		logger.Error(jobErr, "error to get spark job")
		err = r.createJob(ctx, logger, &sparkJob, rayCluster)
		if err != nil {
			logger.Error(err, "failed to create the spark job in spark operator")
			sparkJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
			sparkJob.Status.Message = fmt.Sprintf("failed to create the spark job in cluster %s/%s", rayCluster.Namespace, rayCluster.Name)
		}
		sparkJob.Status.State = v2pb.RAY_JOB_STATE_INITIALIZING
		res.RequeueAfter = requeueAfter
	} else if status != nil {
		// if the job has created, we keep checking the status to see if it reaches the final state
		if r.isTerminatedState(*status) {
			logger.Info("job finished with status", "status", *status)
			sparkJob.Status.JobStatus = string(*status)
			if *status == "SUCCEEDED" {
				sparkJob.Status.State = v2pb.RAY_JOB_STATE_SUCCEEDED
			} else if *status == "FAILED" {
				sparkJob.Status.State = v2pb.RAY_JOB_STATE_FAILED
			} else if *status == "STOPPED" {
				sparkJob.Status.State = v2pb.RAY_JOB_STATE_KILLED
			}
			if jobFailedReason != nil {
				sparkJob.Status.Message = string(*jobFailedReason)
			}
		} else {
			// job is still running, wait
			logger.Info("job is running")
			sparkJob.Status.State = v2pb.RAY_JOB_STATE_RUNNING
			res.RequeueAfter = requeueAfter
		}
	} else {
		// invalid status, we requeue
		logger.Info("unknown status, re-queuing")
		res.RequeueAfter = requeueAfter
	}

	if !reflect.DeepEqual(originalRayJob, sparkJob) {
		// update the resource in ETCD
		if r.isRayJobTerminatedState(sparkJob.Status.State) {
			utils.MarkImmutable(&sparkJob)
		}
		err := r.Status().Update(ctx, &sparkJob)
		if err != nil {
			logger.Error(err, "failed to update status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
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
	sparkJob := &v1.RayJob{
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
				"spark.io/cluster":    cluster.Name,
				"rayClusterNamespace": cluster.Namespace,
			},
			Entrypoint: job.Spec.Entrypoint,
		},
	}

	createdRayJob, err := r.RayJobs(cluster.Namespace).Create(ctx, sparkJob, metav1.CreateOptions{})
	job.Status.JobId = createdRayJob.Status.JobId
	job.Status.DashboardUrl = createdRayJob.Status.DashboardURL
	job.Status.JobDeploymentStatus = string(createdRayJob.Status.JobDeploymentStatus)
	log.Info("spark job created", "namespace", createdRayJob.Namespace, "name", createdRayJob.Name)
	if err != nil {
		log.Error(err, "Failed to submit RayJob")
		return err
	}

	return nil
}

func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, sparkJob *v2pb.SparkJob) (*v1.JobStatus, *v1.JobFailedReason, error) {
	var app sparkv1beta2.SparkApplication
	err := r.Get(ctx, sparkJob.Name, app, metav1.GetOptions{})
	// Fetch the status of the RayJob
	if err != nil {
		logger.Error(err, "failed to get RayJob status: %v")
		return nil, nil, err
	}
	sparkJob.Status.JobId = rayV1Job.Status.JobId
	sparkJob.Status.DashboardUrl = rayV1Job.Status.DashboardURL
	sparkJob.Status.JobDeploymentStatus = string(rayV1Job.Status.JobDeploymentStatus)

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
