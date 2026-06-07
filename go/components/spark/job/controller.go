// Package job implements a Kubernetes controller for managing SparkJob resources.
//
// This package provides a reconciler that manages Spark jobs executing on Kubernetes
// via the Spark Operator. SparkJob resources represent distributed data processing
// jobs running on Apache Spark, with automatic creation and monitoring of
// SparkApplication custom resources.
//
// Job Lifecycle:
//
// SparkJob resources progress through the following states:
//   - NOT_FOUND: SparkApplication doesn't exist, will be created
//   - SUBMITTED/RUNNING: Job is executing on Spark cluster
//   - COMPLETED/FAILED: Terminal states after job completion
//
// Integration:
//
//   - Spark Operator: Creates and monitors SparkApplication CRDs
//   - Spark Client: Interfaces with Spark Operator for job management
//   - SparkApplication: Underlying resource that manages Spark driver and executor pods
//
// The controller continuously polls SparkApplication status and updates the local
// SparkJob resource with current state and conditions.
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
	constants "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	jobsutils "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// requeueAfter defines the delay before retrying reconciliation.
	requeueAfter = 10 * time.Second

	// sparkAppState* constants mirror the Spark Operator's ApplicationStateType strings.
	// The Spark Operator returns these as plain strings from its status field; using
	// named constants here prevents silent breakage if the comparison strings drift.
	sparkAppStateRunning   = "RUNNING"
	sparkAppStateCompleted = "COMPLETED"
	sparkAppStateFailed    = "FAILED"
)

// Reconciler manages the lifecycle of SparkJob custom resources.
//
// All fields are unexported. Exported struct fields become permanent public API surface —
// external packages can depend on them directly, making future removal a breaking change.
// Use NewReconciler() to construct instances.
//
// The reconciler ensures Spark jobs are submitted to the Spark Operator and monitors
// their execution status. It handles job creation via the Spark client and continuously
// polls SparkApplication resources for status updates.
//
// Key responsibilities:
//   - Creating SparkApplication resources when SparkJob is submitted
//   - Monitoring SparkApplication status (SUBMITTED, RUNNING, COMPLETED, FAILED)
//   - Updating SparkJob status conditions based on application state
//   - Handling job failures and error messages
type Reconciler struct {
	client.Client             // Kubernetes client for local operations
	sparkClient   Client      // Client for Spark Operator interactions
	env           env.Context // Environment configuration context
}

// NewReconciler creates a new SparkJob reconciler with the required dependencies.
func NewReconciler(c client.Client, sparkClient Client, env env.Context) *Reconciler {
	return &Reconciler{
		Client:      c,
		sparkClient: sparkClient,
		env:         env,
	}
}

// Reconcile implements the Kubernetes reconciliation loop for SparkJob resources.
//
// This method handles the complete job lifecycle:
//  1. Retrieve SparkJob resource
//  2. Check if SparkApplication exists via Spark client
//  3. Create SparkApplication if not found
//  4. Poll SparkApplication status and update conditions
//  5. Update SparkJob status with current state
//
// State mapping from SparkApplication to SparkJob conditions:
//   - RUNNING → SparkAppRunningCondition = TRUE
//   - COMPLETED → SucceededCondition = TRUE
//   - FAILED → SucceededCondition = FALSE (with error message)
//
// Returns ctrl.Result with RequeueAfter for ongoing monitoring, or an error
// if reconciliation should be retried.
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

	// If termination is requested, terminate the SparkApplication and drive
	// the SparkJob to a terminal state.
	if sparkJob.Spec.GetTermination().GetType() != v2pb.TERMINATION_TYPE_INVALID {
		return r.handleTermination(ctx, logger, &sparkJob)
	}

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
		switch *stateStr {
		case sparkAppStateRunning:
			setCondition(&sparkJob.Status.StatusConditions, constants.SparkAppRunningCondition, apipb.CONDITION_STATUS_TRUE, "Spark application is running", "Running")
		case sparkAppStateCompleted:
			setCondition(&sparkJob.Status.StatusConditions, constants.SucceededCondition, apipb.CONDITION_STATUS_TRUE, "Spark job succeeded", "Succeeded")
		case sparkAppStateFailed:
			// Use the error message from SparkApplication if available, otherwise use a default
			failureMessage := "Spark job failed"
			if errorMessage != "" {
				failureMessage = errorMessage
			}
			setCondition(&sparkJob.Status.StatusConditions, constants.SucceededCondition, apipb.CONDITION_STATUS_FALSE, failureMessage, "Failed")
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

// Register registers the SparkJob controller with the controller manager.
//
// This method configures the controller to watch SparkJob custom resources and
// trigger reconciliation when they are created, updated, or deleted.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.SparkJob{}).
		Complete(r)
}

// setCondition sets or updates a condition in the status conditions slice.
//
// This function manages SparkJob status conditions by:
//   - Adding a new condition if it doesn't exist
//   - Updating an existing condition if status, message, or reason changed
//   - Preserving existing condition if all fields match
//
// Returns true if the condition was added or updated, false if no change was needed.
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

// createJob creates a new SparkApplication for the given SparkJob.
//
// This method delegates to the Spark client to create a SparkApplication custom
// resource in the cluster. The Spark Operator then provisions driver and executor
// pods to run the Spark job.
//
// Returns an error if creation fails.
func (r *Reconciler) createJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error {
	return r.sparkClient.CreateJob(ctx, log, job)
}

// getJobStatus retrieves the current status of a SparkApplication.
//
// This method polls the Spark Operator for the current state of the job.
//
// Returns:
//   - State string pointer (e.g., "RUNNING", "COMPLETED", "FAILED")
//   - Job URL for accessing Spark UI
//   - Error message if job failed
//   - Error if status retrieval fails
func (r *Reconciler) getJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, string, error) {
	return r.sparkClient.GetJobStatus(ctx, logger, job)
}

// isSparkJobKilled reports whether the SparkJob has already reached the killed
// terminal state, so termination handling is idempotent across reconciles.
func isSparkJobKilled(job *v2pb.SparkJob) bool {
	for _, cond := range job.Status.StatusConditions {
		if cond.Type == constants.KilledCondition && cond.Status == apipb.CONDITION_STATUS_TRUE {
			return true
		}
	}
	return false
}

// getSuccessFromTerminateSpec derives the Succeeded condition status from the
// requested termination type: an explicit SUCCEEDED termination resolves to TRUE,
// anything else (FAILED, etc.) to FALSE.
func getSuccessFromTerminateSpec(job *v2pb.SparkJob) apipb.ConditionStatus {
	if job.Spec.Termination.GetType() == v2pb.TERMINATION_TYPE_SUCCEEDED {
		return apipb.CONDITION_STATUS_TRUE
	}
	return apipb.CONDITION_STATUS_FALSE
}

// handleTermination terminates the underlying SparkApplication and drives the
// SparkJob to a terminal state, marking it immutable.
//
// Deleting the SparkApplication is best-effort: a not-found error means the
// workload is already gone and is treated as success.
func (r *Reconciler) handleTermination(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (ctrl.Result, error) {
	res := ctrl.Result{}

	// Already terminal - nothing left to do.
	if isSparkJobKilled(job) {
		return res, nil
	}

	if err := r.sparkClient.DeleteJob(ctx, logger, job); err != nil && !utils.IsNotFoundError(err) {
		logger.Error(err, "failed to terminate SparkApplication",
			"operation", "terminate_job",
			"namespace", job.Namespace,
			"name", job.Name)
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("terminate spark job %q/%q: %w", job.Namespace, job.Name, err)
	}

	message := "Spark job killed"
	if job.Spec.Termination.Reason != "" {
		message = job.Spec.Termination.Reason
	}

	// Drive the SparkJob to its terminal state using the shared condition helpers:
	// set Killed and the Succeeded condition derived from the termination type, so
	// consumers have a definite outcome for the terminated job.
	killed := jobsutils.GetCondition(&job.Status.StatusConditions, constants.KilledCondition, job.Generation)
	jobsutils.UpdateCondition(killed, jobsutils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Generation: job.Generation,
		Reason:     constants.SparkAppKilled,
		Message:    message,
	})
	succeeded := jobsutils.GetCondition(&job.Status.StatusConditions, constants.SucceededCondition, job.Generation)
	jobsutils.UpdateCondition(succeeded, jobsutils.ConditionUpdateParams{
		Status:     getSuccessFromTerminateSpec(job),
		Generation: job.Generation,
		Reason:     job.Spec.Termination.Reason,
	})

	// Mark the SparkJob immutable so it cannot transition further once killed.
	// Update() persists the annotation; Status().Update() persists the status
	// conditions. Both are needed because the CRD has a status subresource.
	utils.MarkImmutable(job)
	if err := r.Update(ctx, job); err != nil {
		logger.Error(err, "failed to update SparkJob",
			"operation", "update",
			"namespace", job.Namespace,
			"name", job.Name)
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("update spark job for %q/%q: %w", job.Namespace, job.Name, err)
	}
	if err := r.Status().Update(ctx, job); err != nil {
		logger.Error(err, "failed to update SparkJob status",
			"operation", "update_status",
			"namespace", job.Namespace,
			"name", job.Name)
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("update spark job status for %q/%q: %w", job.Namespace, job.Name, err)
	}

	logger.Info("SparkJob terminated", "name", job.Name, "namespace", job.Namespace)
	return res, nil
}
