package scheduler

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	sched "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/metrics"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"

	"github.com/go-logr/logr"
	protoTypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/components/libs/goroutine"
	"github.com/uber-go/tally"
	"go.uber.org/fx"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobQueue is the job queue for the scheduler.
type JobQueue interface {
	Enqueue(ctx context.Context, job matypes.SchedulableJob) error
}

// Scheduler schedules jobs based on the assignment engine.
type Scheduler struct {
	api.Handler
	log                logr.Logger
	mgr                ctrl.Manager
	metrics            *metrics.ControllerMetrics
	assignmentStrategy framework.AssignmentStrategy

	scheduleFunc  scheduleFunc
	internalQueue sched.Queue

	// guard against usage before full initialization
	initLock atomic.Bool
}

var _ JobQueue = (*Scheduler)(nil)

// Params is the input to the constructor
type Params struct {
	fx.In

	Manager            ctrl.Manager
	Queue              sched.Queue
	ClusterCache       cluster.RegisteredClustersCache
	Scope              tally.Scope
	APIHandlerFactory  apiHandler.Factory
	AssignmentStrategy framework.AssignmentStrategy
}

const (
	_controllerName = "scheduler"
	_assignJobError = "assign_job_error"
)

// for testing
type scheduleFunc func(context.Context) error

// NewController returns a new Controller.
func NewScheduler(p Params) *Scheduler {
	log := p.Manager.GetLogger().WithValues(constants.Component, _controllerName)
	handler, err := p.APIHandlerFactory.GetAPIHandler(p.Manager.GetClient())
	utilruntime.Must(err)

	scheduler := newScheduler(p, log, handler)
	scheduler.init()

	return scheduler
}

func newScheduler(p Params, log logr.Logger, handler api.Handler) *Scheduler {
	c := &Scheduler{
		mgr:                p.Manager,
		Handler:            handler,
		log:                log,
		internalQueue:      p.Queue,
		metrics:            metrics.NewControllerMetrics(p.Scope, _controllerName),
		assignmentStrategy: p.AssignmentStrategy,
	}

	// default schedule func
	c.scheduleFunc = c.scheduleJobsForever
	return c
}

// init sets up the scheduler.
func (c *Scheduler) init() chan any {
	errChan := make(chan any, 1)
	c.log.Info("Initializing scheduler")

	// handle panic
	goroutine.SafeExecute(
		// routine
		func() {
			<-c.mgr.Elected() // only run on the leader

			c.initLock.Store(true)
			c.log.Info("Initialized scheduler on the leader")

			utilruntime.Must(c.run(context.Background()))
		},
		// recover
		func(v any) {
			c.metrics.MetricsScope.Counter(_schedulerLoopExitedCount).Inc(1) // trigger alert
			c.log.Error(fmt.Errorf("%+v", v), "Job scheduler loop panicked", "trace", string(debug.Stack()))
			errChan <- v
		})

	return errChan
}

const _enqueueTimeOut = 1 * time.Second

// metrics
const (
	_schedulingLatency = "scheduling_latency"

	_jobEnqueueFailureCount   = "job.enqueue_failed_count"
	_jobEnqueueSuccessCount   = "job.enqueue_success_count"
	_schedulerLoopExitedCount = "loop_exited_count"
	_schedulerNotInitialized  = "scheduler_not_initialized"
	_schedulerLoopReturnCount = "loop_return_count"
	_scheduleJobFailureCount  = "job_failed_count"
	_scheduleJobSuccessCount  = "job_success_count"
	_schedulerQueueLength     = "queue_length"

	_jobTypeTag         = "job_type"
	_assignedClusterTag = "assigned_cluster"
)

// Enqueue enqueues the job in the scheduler.
func (c *Scheduler) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
	if !c.initLock.Load() {
		c.metrics.MetricsScope.Counter(_schedulerNotInitialized).Inc(1)
		return fmt.Errorf("enqueue err:%v", _schedulerNotInitialized)
	}

	qc, cancel := context.WithTimeout(ctx, _enqueueTimeOut)
	defer cancel()

	if err := c.internalQueue.Add(qc, job); err != nil {
		if err == matypes.ErrJobAlreadyExists {
			return fmt.Errorf("enqueue err:%w", err)
		}
		c.metrics.MetricsScope.Counter(_jobEnqueueFailureCount).Inc(1)
		return fmt.Errorf("enqueue err:%v, queue length:%v", err, c.internalQueue.Length())
	}
	c.metrics.MetricsScope.Counter(_jobEnqueueSuccessCount).Inc(1)
	return nil
}

// Run the job scheduler
func (c *Scheduler) run(ctx context.Context) error {
	// schedule jobs
	err := c.scheduleFunc(ctx)
	if err != nil {
		return err
	}

	c.metrics.MetricsScope.Counter(_schedulerLoopReturnCount).Inc(1)
	return nil
}

// scheduleJobsForever runs the scheduler loop
func (c *Scheduler) scheduleJobsForever(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			scheduleNextFunc := func() error {
				c.metrics.MetricsScope.Gauge(_schedulerQueueLength).Update(float64(c.internalQueue.Length()))

				// fetch from queue and generate assignment
				ct, cancel := context.WithTimeout(ctx, time.Minute)
				defer cancel()

				// start time for timer to capture scheduling latency
				startTime := time.Now().Unix()

				poppedJob, err := c.internalQueue.Get(ct)
				successTags := map[string]string{}
				if err != nil {
					c.log.V(1).Info("Nothing to pull from the scheduler queue", "pop_error", err.Error())
					return nil
				}
				defer c.internalQueue.Done(ct, poppedJob)

				// re-fetch the job to avoid resource version conflict with updates in this controller
				var job framework.BatchJob
				if err := c.fetchLatestJob(ctx, poppedJob, &job); err != nil {
					return fmt.Errorf("failed to fetch latest job for job name:%s namespace:%s err:%v", poppedJob.GetName(), poppedJob.GetNamespace(), err)
				}

				if err := c.assignJob(ctx, job); err != nil {
					if errors.Is(err, utils.ErrStatusUpdate) {
						// Status update in assignment can fail due to various reasons like conflict
						// or connection errors. These error will be ignored because and let the scheduler will retry
						// this job in the next scheduling cycle for this job.
						return nil
					}

					c.metrics.MetricsScope.Tagged(
						map[string]string{
							constants.FailureReasonKey: _assignJobError,
							_jobTypeTag:                strings.ToLower(job.GetObject().GetObjectKind().GroupVersionKind().Kind),
						}).Counter(_scheduleJobFailureCount).Inc(1)
					return fmt.Errorf("failed assignment for job name:%s namespace:%s err:%v", job.GetName(), job.GetNamespace(), err)
				}

				successTags = map[string]string{
					_jobTypeTag: strings.ToLower(job.GetObject().GetObjectKind().GroupVersionKind().Kind),
				}

				c.metrics.MetricsScope.Tagged(successTags).Counter(_scheduleJobSuccessCount).Inc(1)
				c.metrics.MetricsScope.Tagged(successTags).Timer(_schedulingLatency).Record(time.Duration(time.Now().Unix() - startTime))

				return nil
			}

			err := scheduleNextFunc()
			if err != nil {
				c.log.Error(err, "Failed to schedule job")
			}
		}
	}
}

func (c *Scheduler) fetchLatestJob(ctx context.Context, job matypes.SchedulableJob, latest *framework.BatchJob) error {
	getCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch job.GetJobType() {
	case matypes.RayCluster:
		rayCluster := &v2pb.RayCluster{}
		if err := c.Get(getCtx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, rayCluster); err != nil {
			return err
		}
		*latest = framework.BatchRayCluster{RayCluster: rayCluster}
		return nil
	case matypes.SparkJob:
		sparkJob := &v2pb.SparkJob{}
		if err := c.Get(getCtx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, sparkJob); err != nil {
			return err
		}
		*latest = framework.BatchSparkJob{SparkJob: sparkJob}
		return nil
	default:
		return fmt.Errorf("unrecognized job type")
	}
}

// getAssignmentInfoForJob gets the assignment info for a given job
func (c *Scheduler) getAssignmentInfoForJob(
	ctx context.Context,
	job framework.BatchJob,
) (*v2pb.AssignmentInfo, bool, error) {
	ai, found, reason, err := c.assignmentStrategy.Select(ctx, job)
	_ = reason
	return ai, found, err
}

func (c *Scheduler) assignJob(ctx context.Context, job framework.BatchJob) error {
	jobNameNamespace := types.NamespacedName{
		Name:      job.GetName(),
		Namespace: job.GetNamespace(),
	}

	log := c.log.WithValues(constants.Job, jobNameNamespace.String())

	// get existing scheduling status
	scheduledCondition := utils.GetCondition(
		job.GetConditions(),
		constants.ScheduledCondition,
		job.GetGeneration())

	// check if already assigned
	if scheduledCondition.Status == apipb.CONDITION_STATUS_TRUE {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	assignmentInfo, found, err := c.getAssignmentInfoForJob(ctx, job)
	if err != nil {
		return err
	}

	if assignmentInfo != nil {
		if err := utils.UpdateStatusWithRetries(ctx, c, job.GetObject(),
			func(obj client.Object) {
				switch j := obj.(type) {
				case *v2pb.RayCluster:
					j.Status.Assignment = assignmentInfo
					cond := utils.GetCondition(&j.Status.StatusConditions, constants.ScheduledCondition, j.GetGeneration())
					utils.UpdateCondition(cond, utils.ConditionUpdateParams{Status: apipb.CONDITION_STATUS_TRUE, Reason: "ClusterMatched"})
				case *v2pb.SparkJob:
					j.Status.Assignment = assignmentInfo
					cond := utils.GetCondition(&j.Status.StatusConditions, constants.ScheduledCondition, j.GetGeneration())
					utils.UpdateCondition(cond, utils.ConditionUpdateParams{Status: apipb.CONDITION_STATUS_TRUE, Reason: "ClusterMatched"})
				}
			}, &metav1.UpdateOptions{
				FieldManager: "assignJobToResourcePoolFoundMatch",
			}); err != nil {
			return err
		}

		log.Info("Job scheduled successfully")
		return nil
	}

	if !found {
		// Didn't find any clusters for assignment
		c.log.Info("No clusters found for assignment")
		c.updateIfChanged(scheduledCondition, utils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Reason:     "NoClustersFoundForAssignment",
			Generation: job.GetGeneration(),
		})
	} else {
		// No cluster matched job requirements
		c.log.Info("No cluster matched job requirements. Updating number of attempts.")
		if err := c.updateConditionWithAttempts(scheduledCondition, utils.ConditionUpdateParams{
			Status:     apipb.CONDITION_STATUS_FALSE,
			Reason:     "NoClusterMatchedRequirements",
			Generation: job.GetGeneration(),
		}); err != nil {
			return fmt.Errorf("could not update number of attemps on scheduled condition. err:%v", err)
		}
	}

	if err := utils.UpdateStatusWithRetries(ctx, c, job.GetObject(),
		func(job client.Object) {
			rayCluster := job.(*v2pb.RayCluster)
			cond := utils.GetCondition(&rayCluster.Status.StatusConditions, constants.ScheduledCondition, rayCluster.GetGeneration())
			*cond = *scheduledCondition
			cond.ObservedGeneration = rayCluster.Generation
		}, &metav1.UpdateOptions{
			FieldManager: "assignJobNoMatch",
		}); err != nil {
		return err
	}
	return nil
}

func (c *Scheduler) updateConditionWithAttempts(cond *apipb.Condition, p utils.ConditionUpdateParams) error {
	// This is a float because the protobuf types API does not have an int version.
	// We convert this to an int where required.
	attempts := 1.0
	if cond.Metadata != nil {
		condMeta := &protoTypes.Struct{}
		if err := protoTypes.UnmarshalAny(cond.Metadata, condMeta); err != nil {
			return fmt.Errorf("condition metadata cannot be read err:%v", err)
		}

		if existingRetry, ok := condMeta.GetFields()[constants.NumSchedulerAttempts]; ok {
			attempts = 1 + existingRetry.GetNumberValue()
		}
	}

	metaFields := map[string]*protoTypes.Value{
		constants.NumSchedulerAttempts: {Kind: &protoTypes.Value_NumberValue{NumberValue: attempts}},
	}

	metaData, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: metaFields})
	if err != nil {
		return fmt.Errorf("condition metadata cannot be set with fields %v err:%v", metaFields, err)
	}

	p.Metadata = metaData
	utils.UpdateCondition(cond, p)
	return nil
}

// Do not update if the status or reason has not changed. This helps with reducing the
// number of update events that needs to be handled by the job controllers. This also helps
// with reducing update conflicts for commands such as job killing.
func (c *Scheduler) updateIfChanged(cond *apipb.Condition, p utils.ConditionUpdateParams) {
	if cond.GetStatus() == p.Status && cond.GetReason() == p.Reason &&
		cond.GetObservedGeneration() == p.Generation {
		return
	}

	utils.UpdateCondition(cond, p)
}
