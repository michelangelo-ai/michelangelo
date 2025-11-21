package job

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const requeueAfter = 10 * time.Second

// Reconciler reconciles SparkJob objects by creating and monitoring Spark applications
// in the cluster and updating their status accordingly.
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

	stateStr, url, errorMessage, err := r.getJobStatus(ctx, logger, &sparkJob)
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("SparkApplication not found, creating new one")
			if err = r.createJob(ctx, logger, &sparkJob); err != nil {
				logger.Error(err, "failed to create SparkApplication",
					"operation", "create_job",
					"namespace", req.Namespace,
					"name", req.Name)
				sparkJob.Status.StatusConditions = nil
				sparkJob.Status.JobUrl = ""
				sparkJob.Status.ApplicationId = ""
				res.RequeueAfter = requeueAfter
				return res, fmt.Errorf("create spark job %q: %w", req.NamespacedName, err)
			}
			sparkJob.Status.JobUrl = ""
			sparkJob.Status.ApplicationId = ""
			res.RequeueAfter = requeueAfter
		} else {
			res.RequeueAfter = requeueAfter
			return res, err
		}
	} else if stateStr != nil {
		logger.Info("Found SparkApplication", "ID", sparkJob.Status.ApplicationId, "status", *stateStr, "errorMessage", errorMessage)
		sparkJob.Status.JobUrl = url
		// go through all the status constants and set to running, succeeded, killed
		/*
			NewState              ApplicationStateType = ""
			SubmittedState        ApplicationStateType = "SUBMITTED"
			RunningState          ApplicationStateType = "RUNNING"
			CompletedState        ApplicationStateType = "COMPLETED"
			FailedState           ApplicationStateType = "FAILED"
			FailedSubmissionState ApplicationStateType = "SUBMISSION_FAILED"
			PendingRerunState     ApplicationStateType = "PENDING_RERUN"
			InvalidatingState     ApplicationStateType = "INVALIDATING"
			SucceedingState       ApplicationStateType = "SUCCEEDING"
			FailingState          ApplicationStateType = "FAILING"
			UnknownState          ApplicationStateType = "UNKNOWN"
		*/
		switch *stateStr {
		case "RUNNING":
			setCondition(&sparkJob.Status.StatusConditions, "SparkAppRunning", apipb.CONDITION_STATUS_TRUE, "Spark application is running", "Running")
		case "COMPLETED":
			setCondition(&sparkJob.Status.StatusConditions, "SparkAppRunning", apipb.CONDITION_STATUS_FALSE, "Spark application completed", "Completed")
			setCondition(&sparkJob.Status.StatusConditions, "Succeeded", apipb.CONDITION_STATUS_TRUE, "Spark job succeeded", "Succeeded")
		case "FAILED":
			setCondition(&sparkJob.Status.StatusConditions, "SparkAppRunning", apipb.CONDITION_STATUS_FALSE, "Spark application failed", "Failed")
			// Use the error message from SparkApplication if available, otherwise use a default
			failureMessage := "Spark job failed"
			if errorMessage != "" {
				failureMessage = errorMessage
			}
			setCondition(&sparkJob.Status.StatusConditions, "Succeeded", apipb.CONDITION_STATUS_FALSE, failureMessage, "Failed")
		}

		res.RequeueAfter = requeueAfter
	} else {
		logger.Info("No status for SparkApplication, retrying")
		res.RequeueAfter = requeueAfter
		return res, nil
	}

	if !reflect.DeepEqual(original, sparkJob) {
		if err := r.Status().Update(ctx, &sparkJob); err != nil {
			logger.Error(err, "failed to update SparkJob status",
				"operation", "update_status",
				"namespace", req.Namespace,
				"name", req.Name)
			res.RequeueAfter = requeueAfter
			return res, fmt.Errorf("update spark job status for %q: %w", req.NamespacedName, err)
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

// setCondition sets or updates a condition in the status conditions slice.
// If a condition with the same type already exists, it updates it only if the status changed.
// Returns true if the condition was added or updated, false if it was already set to the same status.
func setCondition(conditions *[]*apipb.Condition, conditionType string, status apipb.ConditionStatus, message string, reason string) bool {
	// Check if condition already exists
	for _, cond := range *conditions {
		if cond.Type == conditionType {
			if cond.Status != status || cond.Message != message || cond.Reason != reason {
				// Update existing condition
				cond.Status = status
				cond.Message = message
				cond.Reason = reason
				return true
			}
			// Condition already exists with same status, message, and reason - no update needed
			return false
		}
	}

	// Condition doesn't exist, add it
	*conditions = append(*conditions, &apipb.Condition{
		Type:    conditionType,
		Status:  status,
		Message: message,
		Reason:  reason,
	})
	return true
}

// createJob creates a new Spark job
func (r *Reconciler) createJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	return r.SparkClient.CreateJob(ctx, log, job)
}

// getJobStatus retrieves the status of the Spark job
func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, string, error) {
	return r.SparkClient.GetJobStatus(ctx, logger, job)
}
