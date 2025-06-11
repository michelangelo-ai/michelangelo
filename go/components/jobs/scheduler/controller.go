package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	sched "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/scheduler"
	matypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework/plugins"
	sharedConstants "code.uber.internal/uberai/michelangelo/shared/constants"
	"code.uber.internal/uberai/michelangelo/shared/libs/goroutine"
	"github.com/go-logr/logr"
	protoTypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobQueue is the job queue for the scheduler.
type JobQueue interface {
	Enqueue(ctx context.Context, job matypes.SchedulableJob) error
}

// ResourcePoolSelector is an interface that selects a resource pool for a given job
type ResourcePoolSelector interface {
	SelectResourcePool(ctx context.Context, job framework.BatchJob) (*v2beta1pb.AssignmentInfo, bool, error)
}

// Controller is the scheduler controller
type Controller struct {
	api.Handler
	log                     logr.Logger
	mgr                     ctrl.Manager
	metrics                 *metrics.ControllerMetrics
	fliprClient             flipr.FliprClient
	fliprConstraintsBuilder matypes.FliprConstraintsBuilder

	scheduleFunc      scheduleFunc
	resourcePoolCache cluster.ResourcePoolCache
	internalQueue     sched.Queue
	filterPlugins     []framework.FilterPlugin
	scorePlugins      []framework.ScorePlugin

	// guard against usage before full initialization
	initLock atomic.Bool
}

var (
	_ JobQueue             = (*Controller)(nil)
	_ ResourcePoolSelector = (*Controller)(nil)
)

// Params is the input to the constructor
type Params struct {
	fx.In

	Manager                 ctrl.Manager
	Queue                   sched.Queue
	ResourcePoolCache       cluster.ResourcePoolCache
	ClusterCache            cluster.RegisteredClustersCache
	OptionBuilder           framework.OptionBuilder
	Scope                   tally.Scope
	APIHandlerFactory       apiHandler.Factory
	FliprClient             flipr.FliprClient
	FliprConstraintsBuilder matypes.FliprConstraintsBuilder
}

const _controllerName = "scheduler"

// for testing
type scheduleFunc func(context.Context) error

// NewController returns a new Controller.
func NewController(p Params) *Controller {
	log := p.Manager.GetLogger().WithValues(constants.Component, _controllerName)
	handler, err := p.APIHandlerFactory.GetAPIHandler(p.Manager.GetClient())
	utilruntime.Must(err)

	queue := newController(p, log, handler)
	queue.init()

	return queue
}

func newController(p Params, log logr.Logger, handler api.Handler) *Controller {
	p.OptionBuilder.Build(
		framework.WithLogger(log),
		framework.WithFlipr(p.FliprClient),
		framework.WithClusterCache(p.ClusterCache),
		framework.WithFliprConstraintsBuilder(matypes.NewFliprConstraintsBuilder()))
	filters, scorers := defaultPlugins(p.OptionBuilder)

	c := &Controller{
		mgr:                     p.Manager,
		Handler:                 handler,
		log:                     log,
		resourcePoolCache:       p.ResourcePoolCache,
		filterPlugins:           filters,
		scorePlugins:            scorers,
		internalQueue:           p.Queue,
		metrics:                 metrics.NewControllerMetrics(p.Scope, _controllerName),
		fliprClient:             p.FliprClient,
		fliprConstraintsBuilder: p.FliprConstraintsBuilder,
	}

	// default schedule func
	c.scheduleFunc = c.scheduleJobsForever
	return c
}

func defaultPlugins(builder framework.OptionBuilder) ([]framework.FilterPlugin, []framework.ScorePlugin) {
	return []framework.FilterPlugin{
			plugins.AffinityFilter{
				OptionBuilder: builder,
			},
			plugins.PoolLimitFilter{
				OptionBuilder: builder,
			},
		},
		[]framework.ScorePlugin{plugins.LoadScorer{
			OptionBuilder: builder,
		}}
}

// init sets up the scheduler.
func (c *Controller) init() chan any {
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
	_fetchLatestJobError          = "fetch_latest_job_error"
	_assignJobToResourcePoolError = "assign_job_to_resource_pool_error"
	_schedulingLatency            = "scheduling_latency"

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
	_resourcePoolTag    = "resource_pool"
)

// Enqueue enqueues the job in the scheduler.
func (c *Controller) Enqueue(ctx context.Context, job matypes.SchedulableJob) error {
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
func (c *Controller) run(ctx context.Context) error {
	// schedule jobs
	err := c.scheduleFunc(ctx)
	if err != nil {
		return err
	}

	c.metrics.MetricsScope.Counter(_schedulerLoopReturnCount).Inc(1)
	return nil
}

// scheduleJobsForever runs the scheduler loop
func (c *Controller) scheduleJobsForever(ctx context.Context) error {
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

				if err := c.assignJobToResourcePool(ctx, job); err != nil {
					if errors.Is(err, utils.ErrStatusUpdate) {
						// Status update in resource pool assignment can fail due to various reasons like conflict
						// or connection errors. These error will be ignored because and let the scheduler will retry
						// this job in the next scheduling cycle for this job.
						return nil
					}

					c.metrics.MetricsScope.Tagged(
						map[string]string{
							constants.FailureReasonKey: _assignJobToResourcePoolError,
							_jobTypeTag:                strings.ToLower(job.GetObject().GetObjectKind().GroupVersionKind().Kind),
						}).Counter(_scheduleJobFailureCount).Inc(1)
					return fmt.Errorf("failed resource pool assignment for job name:%s namespace:%s err:%v", job.GetName(), job.GetNamespace(), err)
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

func (c *Controller) fetchLatestJob(ctx context.Context, job matypes.SchedulableJob, latest *framework.BatchJob) error {
	getCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch job.GetJobType() {
	case matypes.SparkJob:
		sparkJob := &v2beta1pb.SparkJob{}
		if err := c.Get(getCtx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, sparkJob); err != nil {
			return err
		}
		*latest = framework.BatchSparkJob{SparkJob: sparkJob}
		return nil

	case matypes.RayJob:
		rayJob := &v2beta1pb.RayJob{}
		if err := c.Get(getCtx, job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, rayJob); err != nil {
			return err
		}
		*latest = framework.BatchRayJob{RayJob: rayJob}
		return nil

	default:
		return fmt.Errorf("unrecognized job type")
	}
}

type resourcePoolPreference int

const (
	// drPreference means the resource pool is based on the projects's DR settings
	drPreference resourcePoolPreference = iota
	// explicit resource pool preference means the resource pool is specified
	// in the job affinity
	explicitPreference

	// preferences based on uown hierarchy
	firstPreference
	secondPreference
	thirdPreference
	fourthPreference
)

var _resourcePoolPreferenceOrder = []resourcePoolPreference{
	drPreference,
	explicitPreference,
	firstPreference,
	secondPreference,
	thirdPreference,
	fourthPreference,
}

// SelectResourcePool selects a resource pool for a given job
func (c *Controller) SelectResourcePool(
	ctx context.Context,
	job framework.BatchJob) (*v2beta1pb.AssignmentInfo, bool, error) {
	namespacedName := types.NamespacedName{
		Name:      job.GetName(),
		Namespace: job.GetNamespace(),
	}

	log := c.log.WithValues(constants.Job, namespacedName.String())

	var assignmentInfo *v2beta1pb.AssignmentInfo
	var reason string
	poolsFound := false

	for _, preference := range _resourcePoolPreferenceOrder {
		poolsInfo, err := c.getResourcePoolsByPreference(job, preference)
		if err != nil {
			c.metrics.IncJobFailure(constants.FailureReasonErrorFetchingResourcePools)
			return nil, false, err
		}

		log := log.WithValues("preference", preference)

		if len(poolsInfo) == 0 {
			log.Info("No pools found by preference; continuing to next")
			continue
		}

		log.Info("Found pools by preference", zapPoolField(poolsInfo))
		poolsFound = true

		if preference == drPreference || preference == explicitPreference {
			assignmentInfo = &v2beta1pb.AssignmentInfo{
				ResourcePool: poolsInfo[0].Pool.Status.Path,
				Cluster:      poolsInfo[0].ClusterName,
			}
			return assignmentInfo, poolsFound, nil
		}

		assignmentInfo, reason, err = c.scheduleJobToResourcePool(ctx, log, job, poolsInfo)
		if err != nil {
			return nil, poolsFound, err
		}

		if assignmentInfo == nil {
			log.Info("Could not schedule job", "reason", reason)
			continue
		}

		return assignmentInfo, poolsFound, nil
	}

	return nil, poolsFound, nil
}

func (c *Controller) assignJobToResourcePool(ctx context.Context, job framework.BatchJob) error {
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
	if scheduledCondition.Status == v2beta1pb.CONDITION_STATUS_TRUE {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	assignmentInfo, poolsFound, err := c.SelectResourcePool(ctx, job)
	if err != nil {
		return err
	}

	if assignmentInfo != nil {
		if err := utils.UpdateStatusWithRetries(ctx, c, job.GetObject(),
			func(job client.Object) {
				switch job.(type) {
				case *v2beta1pb.SparkJob:
					sparkJob := job.(*v2beta1pb.SparkJob)
					sparkJob.Status.Assignment = assignmentInfo
					cond := utils.GetCondition(&sparkJob.Status.StatusConditions, constants.ScheduledCondition, sparkJob.GetGeneration())
					utils.UpdateCondition(cond, utils.ConditionUpdateParams{
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
						Reason: "Peloton Cluster is specified",
					})
				case *v2beta1pb.RayJob:
					rayJob := job.(*v2beta1pb.RayJob)
					rayJob.Status.Assignment = assignmentInfo
					cond := utils.GetCondition(&rayJob.Status.StatusConditions, constants.ScheduledCondition, rayJob.GetGeneration())
					utils.UpdateCondition(cond, utils.ConditionUpdateParams{
						Status: v2beta1pb.CONDITION_STATUS_TRUE,
						Reason: constants.ResourcePoolMatchedBasedOnLoad,
					})
				}
			}, &metav1.UpdateOptions{
				FieldManager: "assignJobToResourcePoolFoundMatch",
			}); err != nil {
			return err
		}

		log.Info("Job scheduled successfully")
		return nil
	}

	if !poolsFound {
		// Didn't find any pools in the cache
		c.log.Info("No resource pools found in the resource pool cache")
		c.updateIfChanged(scheduledCondition, utils.ConditionUpdateParams{
			Status:     v2beta1pb.CONDITION_STATUS_FALSE,
			Reason:     constants.NoResourcePoolsFoundInCache,
			Generation: job.GetGeneration(),
		})
	} else {
		// No resource pool matched job requirements
		c.log.Info("No resource pool matched job requirements. Updating number of attempts.")
		if err := c.updateConditionWithAttempts(scheduledCondition, utils.ConditionUpdateParams{
			Status:     v2beta1pb.CONDITION_STATUS_FALSE,
			Reason:     constants.NoResourcePoolMatchedRequirements,
			Generation: job.GetGeneration(),
		}); err != nil {
			return fmt.Errorf("could not update number of attemps on scheduled condition. err:%v", err)
		}
	}

	if err := utils.UpdateStatusWithRetries(ctx, c, job.GetObject(),
		func(job client.Object) {
			rayJob := job.(*v2beta1pb.RayJob)
			cond := utils.GetCondition(&rayJob.Status.StatusConditions, constants.ScheduledCondition, rayJob.GetGeneration())
			*cond = *scheduledCondition
			cond.ObservedGeneration = rayJob.Generation
		}, &metav1.UpdateOptions{
			FieldManager: "assignJobToResourcePoolNoMatch",
		}); err != nil {
		return err
	}
	return nil
}

func (c *Controller) updateConditionWithAttempts(cond *v2beta1pb.Condition, p utils.ConditionUpdateParams) error {
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
func (c *Controller) updateIfChanged(cond *v2beta1pb.Condition, p utils.ConditionUpdateParams) {
	if cond.GetStatus() == p.Status && cond.GetReason() == p.Reason &&
		cond.GetObservedGeneration() == p.Generation {
		return
	}

	utils.UpdateCondition(cond, p)
}

func (c *Controller) scheduleJobToResourcePool(
	ctx context.Context,
	log logr.Logger,
	job framework.BatchJob,
	poolsInfo []*cluster.ResourcePoolInfo) (
	assignment *v2beta1pb.AssignmentInfo, reason string, err error) {
	pluginCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// filter
	for _, filter := range c.filterPlugins {
		log.Info("Running scheduling filter", "filter_name", filter.Name())
		poolsInfo, err = filter.Filter(pluginCtx, job, poolsInfo)
		if err != nil {
			return nil, "", fmt.Errorf("filter:%s err:%v", filter.Name(), err)
		}
	}

	if len(poolsInfo) == 0 {
		return nil, "no resource pool could fit the job requirements", nil
	}

	log.Info("Result after filtering", zapPoolField(poolsInfo))

	// score
	for _, scorer := range c.scorePlugins {
		log.Info("Running scheduling scorer", "filter_name", scorer.Name())
		poolsInfo, err = scorer.Score(pluginCtx, job, poolsInfo)
		if err != nil {
			return nil, "", fmt.Errorf("scoring plugin:%s err:%v", scorer.Name(), err)
		}
	}

	log.Info("Result after scoring", zapPoolField(poolsInfo))

	// pick the best one
	poolInfo := poolsInfo[0]

	log.Info("Job assigned to resource pool",
		"resource_pool", poolInfo.Pool.Status.Path,
		"cluster", poolInfo.ClusterName)
	return &v2beta1pb.AssignmentInfo{
		ResourcePool: poolInfo.Pool.Status.Path,
		Cluster:      poolInfo.ClusterName,
	}, "", nil
}

func (c *Controller) getResourcePoolsByPreference(
	job framework.BatchJob, preference resourcePoolPreference) (
	[]*cluster.ResourcePoolInfo, error) {
	jobTeamUUID := job.GetLabels()[constants.UOwnLabelKey]
	switch preference {
	case drPreference:
		pool, err := c.getDRResourcePool(job)
		if err == nil && pool != nil {
			return []*cluster.ResourcePoolInfo{pool}, nil
		}
		return nil, err
	case explicitPreference:
		pool, err := c.getExplicitResourcePool(job)
		if err == nil && pool != nil {
			return []*cluster.ResourcePoolInfo{pool}, nil
		}
		return nil, err
	case firstPreference:
		return c.resourcePoolCache.GetOwnedResourcePools(jobTeamUUID)
	case secondPreference:
		return c.resourcePoolCache.GetAuthorizedResourcePools(jobTeamUUID)
	case thirdPreference:
		return c.resourcePoolCache.GetParentOwnedResourcePools(jobTeamUUID)
	case fourthPreference:
		return c.resourcePoolCache.GetDefaultResourcePools()
	}

	return nil, fmt.Errorf("unknown resource pool preference: %v", preference)
}

const (
	_projectNameKey  = "project_name"
	_runnableNameKey = "runnable_name"
	_jobTypeKey      = "job_type"
)

const (
	_fliprDRRoutingKey = "michelangelo.dr.routing"
)

// DRRoutingConfig is the struct for the routing config for DR
type DRRoutingConfig struct {
	// Target is the target infra environment
	// For spark this is the region provider pair eg: dca-gcp
	// For ray this is the cluster name eg: dca60-kubernetes-batch01 (Once ray moves to federator then we can consolidate this)
	Target string `json:"target"`
	// ResourcePool is the resource pool path
	ResourcePool string `json:"resourcePool"`
}

// getDRResourcePool gets the resource pool from flipr
func (c *Controller) getDRResourcePool(job framework.BatchJob) (*cluster.ResourcePoolInfo, error) {
	// TODO: check if DR is enabled

	// constraints map for flipr check
	constraintsMap := make(map[string]interface{})

	annotations := job.GetAnnotations()
	runnable := annotations[sharedConstants.RunnableNameAnnotation]
	projectName := job.GetNamespace()

	constraintsMap[_runnableNameKey] = runnable
	constraintsMap[_projectNameKey] = projectName
	constraintsMap[_jobTypeKey] = getJobType(job)

	fliprConstraints := c.fliprConstraintsBuilder.GetFliprConstraints(constraintsMap)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	rawValue, err := c.fliprClient.GetValueWithConstraints(ctx, _fliprDRRoutingKey, fliprConstraints)
	if err != nil {
		return nil, fmt.Errorf("flipr could not be queried, err: %v", err)
	}
	bytes, err := json.Marshal(rawValue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal flipr value, err: %v", err)
	}

	var routingConfig DRRoutingConfig
	if err := json.Unmarshal(bytes, &routingConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flipr value, err: %v", err)
	}

	if routingConfig.ResourcePool == "" {
		c.log.Info("No resource pool found for DR routing", "constraints", constraintsMap)
		return nil, nil
	}

	c.log.Info("Resource pool found for DR routing", "resource_pool", routingConfig.ResourcePool, "target", routingConfig.Target)

	return &cluster.ResourcePoolInfo{
		ClusterName: routingConfig.Target,
		Pool: infraCrds.ResourcePool{
			Status: infraCrds.ResourcePoolStatus{
				Path: routingConfig.ResourcePool,
			},
		},
	}, nil
}

func getJobType(job framework.BatchJob) string {
	if job.GetJobType() == matypes.SparkJob {
		return "spark"
	}
	return "ray"
}

func (c *Controller) getExplicitResourcePool(job framework.BatchJob) (*cluster.ResourcePoolInfo, error) {
	if job.GetJobType() != matypes.SparkJob {
		return nil, nil
	}

	// for spark jobs we can get the resource pool from the affinity selector
	affinitySelector := job.GetAffinity().GetResourceAffinity().GetSelector()
	clusterName := affinitySelector.MatchLabels[sharedConstants.RegionProviderAnnotation]
	if clusterName == "" {
		clusterName = affinitySelector.MatchLabels[sharedConstants.ClusterAnnotation]
	}
	poolPath := affinitySelector.MatchLabels[sharedConstants.ResourcePoolPathAnnotation]
	return &cluster.ResourcePoolInfo{
		ClusterName: clusterName,
		Pool: infraCrds.ResourcePool{
			Status: infraCrds.ResourcePoolStatus{
				Path: poolPath,
			},
		},
	}, nil
}

func addAssignmentInfo(assignment *v2beta1pb.AssignmentInfo, poolInfo *cluster.ResourcePoolInfo) {
	assignment.ResourcePool = poolInfo.Pool.Status.Path
	assignment.Cluster = poolInfo.ClusterName
}

func zapPoolField(poolsInfo []*cluster.ResourcePoolInfo) zap.Field {
	return zap.Array("pool", zapcore.ArrayMarshalerFunc(func(ae zapcore.ArrayEncoder) error {
		for _, pool := range poolsInfo {
			ae.AppendString(pool.String())
		}
		return nil
	}))
}
