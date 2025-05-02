package job

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const requeueAfter = 10 * time.Second

type Reconciler struct {
	client.Client
	SparkClient Client
	env         env.Context
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	res := ctrl.Result{}

	var sparkJob v2pb.SparkJob
	if err := r.Get(ctx, req.NamespacedName, &sparkJob); err != nil {
		if utils.IsNotFoundError(err) {
			return res, nil
		}
		res.RequeueAfter = requeueAfter
		return res, err
	}
	original := sparkJob.DeepCopy()

	status, message, err := r.getJobStatus(ctx, logger, &sparkJob)
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("SparkApplication not found, creating new one")
			if err = r.createJob(ctx, logger, &sparkJob); err != nil {
				logger.Error(err, "failed to create SparkApplication")
				sparkJob.Status.StatusConditions = nil
				sparkJob.Status.JobUrl = ""
				sparkJob.Status.ApplicationId = ""
				res.RequeueAfter = requeueAfter
				return res, err
			}
			sparkJob.Status.JobUrl = ""
			sparkJob.Status.ApplicationId = ""
			res.RequeueAfter = requeueAfter
		} else {
			res.RequeueAfter = requeueAfter
			return res, err
		}
	} else if status != nil {
		logger.Info("Found SparkApplication", "ID", sparkJob.Status.ApplicationId, "status", *status)
		sparkJob.Status.JobUrl = message
		sparkJob.Status.ApplicationId = *status
		res.RequeueAfter = requeueAfter
	} else {
		logger.Info("No status for SparkApplication, retrying")
		res.RequeueAfter = requeueAfter
		return res, nil
	}

	if !reflect.DeepEqual(original, sparkJob) {
		if err := r.Status().Update(ctx, &sparkJob); err != nil {
			logger.Error(err, "failed to update SparkJob status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	logger.Info("SparkJob reconciled", "name", sparkJob.Name, "namespace", sparkJob.Namespace)

	return res, nil
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.SparkJob{}).
		Complete(r)
}

// createJob creates a new Spark job
func (r *Reconciler) createJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	return r.SparkClient.CreateJob(ctx, log, job)
}

// getJobStatus retrieves the status of the Spark job
func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, error) {
	return r.SparkClient.GetJobStatus(ctx, logger, job)
}
