// Package kueue implements the Kueue-backed job admission backend
// (jobs.scheduler.backend: kueue). It validates, at enqueue time, that a job
// headed for a Kueue-managed cluster has an existing LocalQueue to land in,
// then hands the job to the default scheduler unchanged. Admission itself
// happens on the compute cluster: the k8s engine labels the dispatched object
// with kueue.x-k8s.io/queue-name and Kueue's own integrations suspend and
// gang-admit it against ClusterQueue quota. The label does not depend on this
// backend: the k8s engine sets it on every object it dispatches to a
// Kueue-managed cluster, whichever backend is configured. Only the LocalQueue
// check is this package's.
//
// The backend gates RayClusters and SparkJobs, and the two are admitted
// differently. Kueue's RayCluster integration is on in its default
// configuration. Its SparkApplication integration is not: a SparkApplication is
// admitted only on a compute cluster that runs Kueue 0.17 or later with the
// SparkApplicationIntegration feature gate on and
// sparkoperator.k8s.io/sparkapplication in integrations.frameworks, and Spark
// Operator 2.4.0 or later watching the namespace the application lands in.
// Without the Kueue prerequisites (version, feature gate, framework) the queue
// label on a SparkApplication is inert: the application starts at once,
// outside any quota. With no Spark Operator watching that namespace the
// application is created but never processed, so the SparkJob gets no
// outcome; on a cluster that does not serve the SparkApplication API at all
// the job is never launched, and the create fails with an error saying so.
// The check in Enqueue reports none of these: it only looks for the
// LocalQueue. See docs/operator-guides/jobs/kueue-scheduler-backend.md.
package kueue

import (
	"context"
	"fmt"
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

	// _validateTimeout bounds the pre-flight's reads: the job fetch, the target
	// prediction and the LocalQueue check.
	_validateTimeout = 30 * time.Second
	// _queueCheckTimeout caps the LocalQueue check within _validateTimeout. A
	// healthy API server answers that GET in milliseconds, and client-go's own
	// TLS-handshake timeout is 10s too.
	_queueCheckTimeout = 10 * time.Second
	// _conditionWriteTimeout bounds the write that records a rejection on the
	// job; see setEnqueuedCondition.
	_conditionWriteTimeout = 10 * time.Second
)

// Compile-time assertion that Spark and Ray are projected into the same
// compute-cluster namespace.
//
// The LocalQueue existence check in Enqueue below hard-codes
// k8sengine.RayLocalNamespace, but that path now also carries Spark jobs,
// whose SparkApplications are projected into k8sengine.SparkLocalNamespace.
// It is correct today only because the two constants name the same namespace.
// Should they ever diverge, this backend would look for a Spark job's
// LocalQueue in the Ray namespace and reject every Spark job as
// KueueQueueNotFound; the fix then is to resolve the namespace from the job's
// own type (the mapper already does, via GetLocalName) rather than naming one
// of them here. Fail the build at that point instead of in production: a map
// literal with a constant key may not repeat it, so the duplicate "false"
// below is a compile error exactly when the condition is false.
//
// That holds only while both namespaces are constants. If either becomes a var
// (to make the Spark namespace configurable, say), the second key is no longer
// constant, repeating a key is legal, and this line compiles and silently
// stops guarding. TestRayAndSparkShareLocalNamespace checks the same equality
// at run time, and keeps checking it after such a change.
var _ = map[bool]struct{}{false: {}, k8sengine.SparkLocalNamespace == k8sengine.RayLocalNamespace: {}}

// Params carries the dependencies of KueueJobQueue. The scheduler module's
// backend factory constructs this directly, so it is a plain struct rather
// than an fx.In.
type Params struct {
	// Delegate is the default scheduler; every job this backend accepts is
	// handed to it unchanged.
	Delegate JobQueue
	// Handler reads and writes job objects/status.
	Handler api.Handler
	// AssignmentStrategy predicts the target cluster (the same strategy the
	// delegate uses for the real assignment).
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

// KueueJobQueue is a JobQueue that validates Kueue queue placement before
// delegating to the default scheduler.
type KueueJobQueue struct {
	delegate JobQueue
	handler  api.Handler
	strategy framework.AssignmentStrategy
	clusters cluster.RegisteredClustersCache
	queues   LocalQueues
	cfg      maconfig.SchedulerConfig
	metrics  *metrics.ControllerMetrics
	log      logr.Logger

	// The pre-flight's deadlines. NewKueueJobQueue sets each from its
	// constant (_validateTimeout and so on); tests shorten them.
	validateTimeout       time.Duration
	queueCheckTimeout     time.Duration
	conditionWriteTimeout time.Duration
}

var _ JobQueue = (*KueueJobQueue)(nil)

// NewKueueJobQueue constructs the Kueue backend.
func NewKueueJobQueue(p Params) *KueueJobQueue {
	return &KueueJobQueue{
		delegate: p.Delegate,
		handler:  p.Handler,
		strategy: p.AssignmentStrategy,
		clusters: p.ClusterCache,
		queues:   p.LocalQueues,
		cfg:      p.Config,
		metrics:  metrics.NewControllerMetrics(p.Scope, _controllerName),
		log:      p.Logger.WithValues(constants.Component, _controllerName),

		validateTimeout:       _validateTimeout,
		queueCheckTimeout:     _queueCheckTimeout,
		conditionWriteTimeout: _conditionWriteTimeout,
	}
}

// Enqueue validates the job's Kueue placement when its target cluster is
// Kueue-managed, then delegates to the default scheduler. Jobs headed for
// non-Kueue clusters (or whose target cannot be predicted yet) pass through
// untouched, which is what allows mixed fleets during migration. A rejection
// is recorded on the job with ctx, not the validation context; see
// setEnqueuedCondition.
func (q *KueueJobQueue) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
	vctx, cancel := context.WithTimeout(ctx, q.validateTimeout)
	defer cancel()

	latest, err := q.fetchLatestJob(vctx, job)
	if err != nil {
		return fmt.Errorf("kueue validation: fetch latest job: %w", err)
	}

	target := q.targetCluster(vctx, latest)
	targetSpec := target.GetSpec()
	if targetSpec.GetSchedulerType() != v2pb.SCHEDULER_TYPE_KUEUE {
		return q.delegate.Enqueue(ctx, job)
	}

	log := q.log.WithValues(constants.Job, latest.GetNamespace()+"/"+latest.GetName(), "cluster", target.GetName())

	project := utils.ProjectNameForJob(latest.GetLabels(), latest.GetNamespace())
	if project == "" {
		// A job with no project identity cannot be mapped to a LocalQueue;
		// fail visibly rather than dispatching into a black hole.
		q.metrics.MetricsScope.Counter(_queueNotFoundCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, latest, constants.KueueQueueNotFound,
			fmt.Sprintf("cannot resolve a LocalQueue on Kueue-managed cluster %q: job has neither a %s label nor a namespace",
				target.GetName(), constants.ProjectNameLabelKey)); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: cannot resolve a project for job %q on Kueue-managed cluster %q",
			latest.GetName(), target.GetName())
	}

	queueName := utils.ResolveLocalQueueName(q.cfg.Kueue, project)
	// A compute cluster that has stopped answering holds this GET until its
	// deadline: the member REST config sets no timeout, and client-go's dialer
	// alone waits 30s, all of the validation time. The cap holds the caller for
	// at most queueCheckTimeout per check instead.
	checkCtx, cancelCheck := context.WithTimeout(vctx, q.queueCheckTimeout)
	defer cancelCheck()
	exists, err := q.queues.Exists(checkCtx, target, k8sengine.RayLocalNamespace, queueName)
	if err != nil {
		q.metrics.MetricsScope.Counter(_queueCheckFailedCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, latest, constants.KueueQueueCheckFailed,
			fmt.Sprintf("checking LocalQueue %q on cluster %q: %v", queueName, target.GetName(), err)); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: check LocalQueue %q on cluster %q: %w", queueName, target.GetName(), err)
	}
	if !exists {
		q.metrics.MetricsScope.Counter(_queueNotFoundCount).Inc(1)
		if uerr := q.setEnqueuedCondition(ctx, latest, constants.KueueQueueNotFound,
			fmt.Sprintf("LocalQueue %q not found in namespace %q on cluster %q (is Kueue installed and the queue created?)",
				queueName, k8sengine.RayLocalNamespace, target.GetName())); uerr != nil {
			log.Error(uerr, "Failed to update Enqueued condition")
		}
		return fmt.Errorf("kueue validation: LocalQueue %q not found on cluster %q", queueName, target.GetName())
	}

	log.Info("Kueue placement validated", "localQueue", queueName)
	return q.delegate.Enqueue(ctx, job)
}

// targetCluster predicts where the job will be assigned: an existing
// assignment wins, otherwise the assignment strategy is consulted. Returns
// nil when no target can be determined — the default path reports that case
// exactly as it does today.
func (q *KueueJobQueue) targetCluster(ctx context.Context, job framework.BatchJob) *v2pb.Cluster {
	if name := job.GetAssignmentInfo().GetCluster(); name != "" {
		return q.clusters.GetCluster(name)
	}
	ai, found, _, err := q.strategy.Select(ctx, job)
	if err != nil || !found || ai.GetCluster() == "" {
		return nil
	}
	return q.clusters.GetCluster(ai.GetCluster())
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
		return nil, fmt.Errorf("unrecognized job type %d", job.GetJobType())
	}
}

// setEnqueuedCondition records a rejection on the job as Enqueued=False. Its
// callers pass Enqueue's own ctx, not the validation context, and the write
// gets conditionWriteTimeout of its own from it: the verdict has to be
// recorded even when the LocalQueue check used up the validation time, which
// would leave the write an expired context. A cancelled caller (the manager
// shutting down) still cancels the write.
func (q *KueueJobQueue) setEnqueuedCondition(ctx context.Context, job framework.BatchJob, reason, message string) error {
	ctx, cancel := context.WithTimeout(ctx, q.conditionWriteTimeout)
	defer cancel()
	return utils.UpdateStatusWithRetries(ctx, q.handler, job.GetObject(),
		func(obj client.Object) {
			switch j := obj.(type) {
			case *v2pb.RayCluster:
				cond := utils.GetCondition(&j.Status.StatusConditions, constants.EnqueuedCondition, j.GetGeneration())
				utils.UpdateCondition(cond, utils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Reason:     reason,
					Generation: j.GetGeneration(),
					Message:    message,
				})
			case *v2pb.SparkJob:
				cond := utils.GetCondition(&j.Status.StatusConditions, constants.EnqueuedCondition, j.GetGeneration())
				utils.UpdateCondition(cond, utils.ConditionUpdateParams{
					Status:     apipb.CONDITION_STATUS_FALSE,
					Reason:     reason,
					Generation: j.GetGeneration(),
					Message:    message,
				})
			}
		}, &metav1.UpdateOptions{
			FieldManager: "kueueQueueValidation",
		})
}
