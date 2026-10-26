// Package spark provides the Kubernetes controller that runs Apache Spark jobs.
//
// A SparkJob runs on a member (compute) cluster as a SparkApplication that the
// Spark Operator on that cluster manages. The control plane does not talk to the
// Spark Operator directly. It goes through the federated jobs control plane: the
// SparkJob is admitted to the jobs scheduler, which assigns it a member cluster,
// and the SparkApplication is created there through the federated jobs client.
// The SparkJob to SparkApplication translation lives in
// go/components/jobs/client/k8sengine/spark.go.
//
// Architecture:
//
// The SparkJob controller in the job package has two parts:
//   - Reconciler: puts a new SparkJob on the scheduler's queue, waits for the
//     scheduler to assign a member cluster, creates the SparkApplication there
//     once, and handles termination and deletion requests. Before it creates
//     the SparkApplication it puts a finalizer on the SparkJob, so a deleted
//     SparkJob stays until its SparkApplication is deleted too. If that delete
//     keeps failing for an hour after the SparkJob's deletion, the Reconciler
//     gives up: it logs an error naming the cluster and the SparkApplication,
//     which may still be running and has to be deleted by hand, and lets the
//     SparkJob go. The finalizer comes off as well once the SparkJob's outcome
//     is recorded, since there is nothing left to stop then. As a backstop for
//     watch events it missed, it also re-checks a launched SparkJob that has no
//     outcome yet once the SparkJob's status has gone 5 minutes without a
//     change; while that status keeps changing, the watch is evidently
//     delivering. It runs ten workers, so it reconciles up to ten SparkJobs
//     at once, and gives each call to a member cluster up to 30 seconds.
//     After a re-check on a member cluster fails, or a create or a delete
//     there runs out of time, it holds every call to that cluster for the
//     next 5 minutes: the SparkJobs there wait for the hold to end, with no
//     call to the cluster and nothing written. Clusters that have stopped
//     answering so cost at most one wave of calls per cluster per hold, on
//     each of them the calls already under way when the first of them fails,
//     no more than one on each of the ten workers, while the other workers go
//     on with the SparkJobs on other clusters. A hold also puts off giving up
//     on a deleted SparkJob, which is decided only after a delete fails, by
//     the rest of each hold the SparkJob meets.
//   - Federated watcher: follows the SparkApplications this control plane
//     created on every ready member cluster and records their state on the
//     SparkJob's status.
//
// Status:
//
//   - A SparkApplication whose creation fails for a reason that may pass, such
//     as a member cluster that is unavailable for now, is created again on the
//     controller's backoff, and the failure is recorded as Launched=UNKNOWN
//     with reason SparkApplicationCreationRetrying. A failure that reads
//     differently replaces the record only once the record is 5 minutes old,
//     so a message that changes on every attempt cannot cut each backoff
//     short. A creation that fails for good records Launched=FALSE and
//     Succeeded=FALSE.
//   - A completed application records Succeeded=TRUE, and a failed one records
//     Succeeded=FALSE.
//   - An application deleted from the member cluster while it is still running
//     records Succeeded=FALSE with a reason saying it was deleted, and no Killed
//     condition. Uniflow treats Killed=TRUE as "do not retry", so Killed is
//     reserved for a termination the user requested. An application deleted
//     after it finished keeps its real outcome, provided the control plane saw
//     it finish before it disappeared, through the watch or a re-check. One
//     that finished and was deleted while the control plane could not see its
//     cluster, during an outage that outlasted the 24-hour TTL the projection
//     gives every application, say, is recorded as deleted like a running one.
//
// Integration:
//
//   - Jobs scheduler: assigns the SparkJob to a member cluster and, when
//     jobs.scheduler.backend is kueue and that cluster is Kueue-managed,
//     checks the project's LocalQueue first
//   - Federated jobs client: creates, reads and deletes the SparkApplication on
//     the member cluster and watches it. On a Kueue-managed cluster, one whose
//     scheduler_type is Kueue, the SparkApplication carries the Kueue queue
//     label whatever jobs.scheduler.backend is
//   - Spark Operator: runs on the member cluster and turns the SparkApplication
//     into driver and executor pods
//
// The standalone Spark Operator client in go/components/spark/job/client is
// deprecated and nothing consumes it. Module still registers it, only so that
// the controller's move to the federated jobs control plane can be reverted on
// its own.
//
// Usage:
//
//	fx.New(
//	    spark.Module,
//	    // other modules...
//	)
package spark

import (
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job/client"
	"go.uber.org/fx"
)

var (
	// Module provides Uber FX dependency injection options for the SparkJob controller.
	//
	// job.Module registers the SparkJob controller and its federated
	// SparkApplication watcher, which run SparkJobs through the federated jobs
	// control plane. client.Module is the standalone Spark Operator client the
	// controller used before that. It is deprecated and nothing consumes it; it
	// stays registered only so that the federated flip can be reverted on its own.
	Module = fx.Options(
		// client.Module is deprecated and nothing consumes it; it stays registered
		// so the SparkJob controller's federated flip can be reverted on its own.
		client.Module,
		job.Module,
	)
)
