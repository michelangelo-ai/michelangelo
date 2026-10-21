// Package kueue implements the Kueue-backed job admission backend
// (jobs.scheduler.backend: kueue). At enqueue time it makes the job's single
// placement decision and persists it: a job headed for a Kueue-managed
// cluster is placed only once it is validated to have an existing LocalQueue
// to land in, and jobs headed for other clusters are placed without
// validation. Admission itself happens on the compute cluster: the k8s engine
// labels the dispatched object with kueue.x-k8s.io/queue-name and Kueue's own
// integrations suspend and gang-admit it against ClusterQueue quota.
package kueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/api"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/metrics"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"github.com/uber-go/tally"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// JobQueue matches scheduler.JobQueue structurally. It is declared locally so
// the scheduler module can import this package (for the backend factory)
// without an import cycle.
type JobQueue interface {
	Enqueue(ctx context.Context, job matypes.SchedulableJob) error
}

const (
	_controllerName = "kueuescheduler"

	_queueNotFoundCount    = "kueue.queue_not_found_count"
	_queueCheckFailedCount = "kueue.queue_check_failed_count"

	_validateTimeout = 30 * time.Second
)

// Params carries the dependencies of KueueJobQueue. The scheduler module's
// backend factory constructs this directly, so it is a plain struct rather
// than an fx.In.
type Params struct {
	// Handler reads and writes job objects/status.
	Handler api.Handler
	// AssignmentStrategy picks the target cluster of a job that has no
	// recorded assignment.
	AssignmentStrategy framework.AssignmentStrategy
	// ClusterCache resolves cluster names to registered clusters.
	ClusterCache cluster.RegisteredClustersCache
	// LocalQueues checks LocalQueue existence on compute clusters.
	LocalQueues LocalQueues
	// Config is the jobs.scheduler configuration.
	Config maconfig.SchedulerConfig
	// Scope emits metrics.
	Scope tally.Scope
	// Logger logs.
	Logger logr.Logger
}

// KueueJobQueue is a JobQueue that makes and persists each job's placement
// decision itself, validating Kueue queue placement first. It replaces the
// default scheduler's decision path, so a validated target is the cluster the
// job is dispatched to.
type KueueJobQueue struct {
	// mu serializes decisions: each one is made and recorded before the next
	// starts.
	mu       sync.Mutex
	handler  api.Handler
	strategy framework.AssignmentStrategy
	clusters cluster.RegisteredClustersCache
	queues   LocalQueues
	cfg      maconfig.SchedulerConfig
	metrics  *metrics.ControllerMetrics
	log      logr.Logger
}

var _ JobQueue = (*KueueJobQueue)(nil)

// NewKueueJobQueue constructs the Kueue backend.
func NewKueueJobQueue(p Params) *KueueJobQueue {
	return &KueueJobQueue{
		handler:  p.Handler,
		strategy: p.AssignmentStrategy,
		clusters: p.ClusterCache,
		queues:   p.LocalQueues,
		cfg:      p.Config,
		metrics:  metrics.NewControllerMetrics(p.Scope, _controllerName),
		log:      p.Logger.WithValues(constants.Component, _controllerName),
	}
}

// Enqueue makes the job's placement decision and records it, unless the job
// is already scheduled. The target is a recorded assignment (objects written
// before decisions were persisted here) or the assignment strategy's choice.
// A Kueue-managed target must have the job's LocalQueue: on failure Enqueued
// is set False with the reason, an error is returned and nothing is recorded.
// Any other target is recorded as is, which is what allows mixed fleets
// during migration. With no target, Scheduled is set False and nil is
// returned so the caller retries later with a fresh decision.
//
// Under this backend Enqueue is the only writer of Enqueued (RFC 0a): True
// with a recorded decision or a no-target result, False with the reason on a
// failed validation. Each is written only when it changes the condition's
// status or reason, so that a waiting job's status is not rewritten on every
// retry and the condition's timestamp is the start of the current wait.
func (q *KueueJobQueue) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	vctx, cancel := context.WithTimeout(ctx, _validateTimeout)
	defer cancel()

	latest, err := q.fetchLatestJob(vctx, job)
	if err != nil {
		return fmt.Errorf("kueue placement: fetch latest job: %w", err)
	}
	if utils.IsJobScheduled(*latest.GetConditions(), latest.GetGeneration()) {
		return nil
	}

	log := q.log.WithValues(constants.Job, latest.GetNamespace()+"/"+latest.GetName())

	target, err := q.targetCluster(vctx, latest, log)
	if err != nil {
		return fmt.Errorf("kueue placement: select a cluster for job %q: %w", latest.GetName(), err)
	}
	if target == nil {
		return q.recordNoTarget(vctx, latest, log)
	}
	log = log.WithValues("cluster", target.GetName())

	targetSpec := target.GetSpec()
	if targetSpec.GetSchedulerType() == v2pb.SCHEDULER_TYPE_KUEUE {
		if err := q.validateLocalQueue(vctx, latest, target, log); err != nil {
			return err
		}
	}
	return q.recordAssignment(vctx, latest, target.GetName(), log)
}

// targetCluster returns the job's placement target: its recorded assignment
// while that cluster is still registered, otherwise the assignment strategy's
// choice. Returns nil when there is no target.
func (q *KueueJobQueue) targetCluster(ctx context.Context, job framework.BatchJob, log logr.Logger) (*v2pb.Cluster, error) {
	if name := job.GetAssignmentInfo().GetCluster(); name != "" {
		if target := q.clusters.GetCluster(name); target != nil {
			return target, nil
		}
		log.Info("Recorded assignment names an unregistered cluster; selecting again", "recordedCluster", name)
	}
	ai, found, err := q.selectCluster(ctx, job, log)
	if err != nil {
		return nil, err
	}
	if !found || ai.GetCluster() == "" {
		return nil, nil
	}
	return q.clusters.GetCluster(ai.GetCluster()), nil
}

// selectCluster returns the assignment strategy's decision. For a job
// re-placed off earlier targets, a strategy that can exclude clusters
// (framework.ExcludingAssignmentStrategy) decides without them; when that
// finds no cluster, it decides again with them, which keeps the outcome a
// decision without the exclusion has: a wait or the same cluster (RFC 0f).
func (q *KueueJobQueue) selectCluster(ctx context.Context, job framework.BatchJob, log logr.Logger) (*v2pb.AssignmentInfo, bool, error) {
	if excluding, ok := q.strategy.(framework.ExcludingAssignmentStrategy); ok {
		state, err := utils.DispatchStateFromCondition(utils.FindCondition(*job.GetConditions(), constants.LaunchedCondition))
		if err != nil {
			log.Error(err, "Ignoring an undecodable dispatch record")
		}
		if len(state.ExcludedClusters) > 0 {
			ai, found, _, err := excluding.SelectExcluding(ctx, job, state.ExcludedClusters)
			if err != nil {
				return nil, false, err
			}
			if found && ai.GetCluster() != "" {
				return ai, true, nil
			}
			log.Info("No cluster left after excluding earlier targets; deciding without the exclusion",
				"excludedClusters", state.ExcludedClusters)
		}
	}
	ai, found, _, err := q.strategy.Select(ctx, job)
	return ai, found, err
}

// validateLocalQueue checks that the job's LocalQueue exists on the
// Kueue-managed target. On failure it sets Enqueued=False with the reason and
// returns an error.
func (q *KueueJobQueue) validateLocalQueue(ctx context.Context, job framework.BatchJob, target *v2pb.Cluster, log logr.Logger) error {
	project := utils.ProjectNameForJob(job.GetLabels(), job.GetNamespace(), q.cfg.Kueue.TrustProjectLabel)
	if project == "" {
		// A job with no project identity cannot be mapped to a LocalQueue;
		// fail visibly rather than dispatching into a black hole.
		q.metrics.MetricsScope.Counter(_queueNotFoundCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, job, constants.KueueQueueNotFound,
			fmt.Sprintf("cannot resolve a LocalQueue on Kueue-managed cluster %q: job has no project identity (a namespace, or a trusted %s label)",
				target.GetName(), constants.ProjectNameLabelKey)); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: cannot resolve a project for job %q on Kueue-managed cluster %q",
			job.GetName(), target.GetName())
	}

	queueName := utils.ResolveLocalQueueName(q.cfg.Kueue, project)
	exists, err := q.queues.Exists(ctx, target, k8sengine.RayLocalNamespace, queueName)
	if err != nil {
		q.metrics.MetricsScope.Counter(_queueCheckFailedCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, job, constants.KueueQueueCheckFailed,
			fmt.Sprintf("checking LocalQueue %q on cluster %q: %v", queueName, target.GetName(), err)); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: check LocalQueue %q on cluster %q: %w", queueName, target.GetName(), err)
	}
	if !exists {
		q.metrics.MetricsScope.Counter(_queueNotFoundCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, job, constants.KueueQueueNotFound,
			fmt.Sprintf("LocalQueue %q not found in namespace %q on cluster %q (is Kueue installed and the queue created?)",
				queueName, k8sengine.RayLocalNamespace, target.GetName())); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: LocalQueue %q not found on cluster %q", queueName, target.GetName())
	}

	log.Info("Kueue placement validated", "localQueue", queueName)
	return nil
}

// recordAssignment persists the decision in one status write, applied to the
// re-read object so that a decision made from a stale read never overwrites
// a newer one.
func (q *KueueJobQueue) recordAssignment(ctx context.Context, job framework.BatchJob, clusterName string, log logr.Logger) error {
	var recorded bool
	if err := utils.UpdateStatusWithRetries(ctx, q.handler, job.GetObject(),
		func(obj client.Object) {
			recorded = recordPlacement(obj, clusterName)
		}, &metav1.UpdateOptions{
			FieldManager: "kueueAssignJob",
		}); err != nil {
		return fmt.Errorf("kueue placement: record assignment of job %q to cluster %q: %w", job.GetName(), clusterName, err)
	}
	if !recorded {
		log.Info("Job was already scheduled by another decision; nothing recorded")
		return nil
	}
	log.Info("Job scheduled successfully")
	return nil
}

// recordNoTarget sets Scheduled=False/NoClustersFoundForAssignment and
// Enqueued=True/AddedToSchedulerQueue, writing only when that changes either
// condition.
func (q *KueueJobQueue) recordNoTarget(ctx context.Context, job framework.BatchJob, log logr.Logger) error {
	log.Info("No clusters found for assignment")
	if !noTargetChangesStatus(*job.GetConditions()) {
		return nil
	}
	if _, err := utils.UpdateStatusIfWithRetries(ctx, q.handler, job.GetObject(), recordNoTargetResult,
		&metav1.UpdateOptions{
			FieldManager: "kueueAssignJobNoMatch",
		}); err != nil {
		return fmt.Errorf("kueue placement: record that job %q has no target cluster: %w", job.GetName(), err)
	}
	return nil
}

// placementStatus returns the status fields a placement decision writes, or
// ok=false for a type that is not placed by this backend.
func placementStatus(obj client.Object) (conds *[]*apipb.Condition, assignment **v2pb.AssignmentInfo, ok bool) {
	switch j := obj.(type) {
	case *v2pb.RayCluster:
		return &j.Status.StatusConditions, &j.Status.Assignment, true
	case *v2pb.SparkJob:
		return &j.Status.StatusConditions, &j.Status.Assignment, true
	default:
		return nil, nil, false
	}
}

// recordPlacement records clusterName as obj's placement decision:
// Status.Assignment, Scheduled=True/ClusterMatched with message
// "cluster <name>", and Enqueued=True/AddedToSchedulerQueue unless Enqueued
// is already True. It changes nothing and returns false when obj is already
// scheduled.
func recordPlacement(obj client.Object, clusterName string) bool {
	conds, assignment, ok := placementStatus(obj)
	if !ok {
		return false
	}
	generation := obj.GetGeneration()
	scheduled := utils.GetCondition(conds, constants.ScheduledCondition, generation)
	if scheduled.GetStatus() == apipb.CONDITION_STATUS_TRUE {
		return false
	}

	*assignment = &v2pb.AssignmentInfo{Cluster: clusterName}
	utils.UpdateCondition(scheduled, utils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Reason:     constants.ClusterMatched,
		Generation: generation,
		Message:    "cluster " + clusterName,
	})
	markEnqueued(conds, generation)
	return true
}

// markEnqueued sets Enqueued=True/AddedToSchedulerQueue unless Enqueued is
// already True.
func markEnqueued(conds *[]*apipb.Condition, generation int64) {
	if utils.FindCondition(*conds, constants.EnqueuedCondition).GetStatus() == apipb.CONDITION_STATUS_TRUE {
		return
	}
	utils.UpdateCondition(utils.GetCondition(conds, constants.EnqueuedCondition, generation), utils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_TRUE,
		Reason:     constants.AddedToSchedulerQueue,
		Generation: generation,
	})
}

// noTargetChangesStatus reports whether recording a no-target result changes
// the conditions: never over a recorded decision; otherwise when Scheduled is
// not already False/NoClustersFoundForAssignment, or Enqueued is not True.
func noTargetChangesStatus(conds []*apipb.Condition) bool {
	scheduled := utils.FindCondition(conds, constants.ScheduledCondition)
	if scheduled.GetStatus() == apipb.CONDITION_STATUS_TRUE {
		return false
	}
	return noTargetChangesScheduled(scheduled) ||
		utils.FindCondition(conds, constants.EnqueuedCondition).GetStatus() != apipb.CONDITION_STATUS_TRUE
}

// recordNoTargetResult records a no-target result on obj when that changes
// its conditions: Scheduled=False/NoClustersFoundForAssignment, and
// Enqueued=True/AddedToSchedulerQueue, since the job waits in the queue for a
// target. It reports whether it changed anything.
func recordNoTargetResult(obj client.Object) bool {
	conds, _, ok := placementStatus(obj)
	if !ok || !noTargetChangesStatus(*conds) {
		return false
	}
	recordNoPlacement(obj)
	markEnqueued(conds, obj.GetGeneration())
	return true
}

// noTargetChangesScheduled reports whether recording "no target" changes the
// Scheduled condition: never over a recorded decision, and not when it
// already says False/NoClustersFoundForAssignment.
func noTargetChangesScheduled(scheduled *apipb.Condition) bool {
	switch {
	case scheduled.GetStatus() == apipb.CONDITION_STATUS_TRUE:
		return false
	case scheduled.GetStatus() == apipb.CONDITION_STATUS_FALSE &&
		scheduled.GetReason() == constants.NoClustersFoundForAssignment:
		return false
	default:
		return true
	}
}

// recordNoPlacement sets obj's Scheduled condition to
// False/NoClustersFoundForAssignment when that changes it, and reports
// whether it did.
func recordNoPlacement(obj client.Object) bool {
	conds, _, ok := placementStatus(obj)
	if !ok {
		return false
	}
	generation := obj.GetGeneration()
	scheduled := utils.GetCondition(conds, constants.ScheduledCondition, generation)
	if !noTargetChangesScheduled(scheduled) {
		return false
	}
	utils.UpdateCondition(scheduled, utils.ConditionUpdateParams{
		Status:     apipb.CONDITION_STATUS_FALSE,
		Reason:     constants.NoClustersFoundForAssignment,
		Generation: generation,
	})
	return true
}

func (q *KueueJobQueue) fetchLatestJob(ctx context.Context, job matypes.SchedulableJob) (framework.BatchJob, error) {
	switch job.GetJobType() {
	case matypes.RayCluster:
		rayCluster := &v2pb.RayCluster{}
		if err := q.handler.Get(ctx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, rayCluster); err != nil {
			return nil, err
		}
		return framework.BatchRayCluster{RayCluster: rayCluster}, nil
	case matypes.SparkJob:
		sparkJob := &v2pb.SparkJob{}
		if err := q.handler.Get(ctx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, sparkJob); err != nil {
			return nil, err
		}
		return framework.BatchSparkJob{SparkJob: sparkJob}, nil
	default:
		return nil, fmt.Errorf("unrecognized job type")
	}
}

// setEnqueuedCondition sets Enqueued=False with the reason of a failed
// validation, writing only when that changes the condition's status or
// reason: a validation that keeps failing the same way writes once.
func (q *KueueJobQueue) setEnqueuedCondition(ctx context.Context, job framework.BatchJob, reason, message string) error {
	if !notEnqueuedChanges(*job.GetConditions(), reason) {
		return nil
	}
	_, err := utils.UpdateStatusIfWithRetries(ctx, q.handler, job.GetObject(),
		func(obj client.Object) bool {
			conds, _, ok := placementStatus(obj)
			if !ok || !notEnqueuedChanges(*conds, reason) {
				return false
			}
			utils.UpdateCondition(utils.GetCondition(conds, constants.EnqueuedCondition, obj.GetGeneration()), utils.ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_FALSE,
				Reason:     reason,
				Generation: obj.GetGeneration(),
				Message:    message,
			})
			return true
		}, &metav1.UpdateOptions{
			FieldManager: "kueueQueueValidation",
		})
	return err
}

// notEnqueuedChanges reports whether Enqueued=False/reason changes the
// conditions: unless Enqueued is already False with that reason.
func notEnqueuedChanges(conds []*apipb.Condition, reason string) bool {
	enqueued := utils.FindCondition(conds, constants.EnqueuedCondition)
	return enqueued.GetStatus() != apipb.CONDITION_STATUS_FALSE || enqueued.GetReason() != reason
}
