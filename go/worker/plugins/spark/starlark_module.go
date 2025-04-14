package spark

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/star"
	"go.starlark.net/starlark"
	"go.uber.org/cadence"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// These are some error reasons
const (
	_errorReasonUnpackArgs           = "UnpackArgsError"
	_errorReasonConvertJob           = "ConvertSparkJobError"
	_errorReasonConvertStarlarkValue = "ConvertStarlarkValueError"
	_errorReasonSubmitJob            = "SubmitJobError"
	_errorReasonSensorJob            = "SensorJobError"
	_errorReasonTermninateJob        = "TerminateJobError"
)

const _reasonForCancel = "Canceled by request"

// These are general const
const (
	_defaultPollSeconds  = 10
	_maxJobSensorRetries = 20
)

// TODO: andrii: implement Spark starlark plugin here

var _ starlark.HasAttrs = (*module)(nil)
var timeout int64 = 0
var poll int64 = 10

type module struct {
	attributes map[string]*starlark.Builtin
	properties map[string]star.PropertyFactory
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]*starlark.Builtin{
		"create_job": starlark.NewBuiltin("create_job", m.createJob),
		"sensor_job": starlark.NewBuiltin("sensor_job", m.sensorJob),
	}
	m.properties = map[string]star.PropertyFactory{
		"running_condition_type":   getRunningConditionType,
		"succeeded_condition_type": getSucceededConditionType,
		"killed_condition_type":    getKilledConditionType,
	}
	return m
}

func (r *module) String() string        { return pluginID }
func (r *module) Type() string          { return pluginID }
func (r *module) Freeze()               {}
func (r *module) Truth() starlark.Bool  { return true }
func (r *module) Hash() (uint32, error) { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) {
	return star.Attr(
		r, n, r.attributes, r.properties)
}
func (r *module) AttrNames() []string { return ext.SortedKeys(r.attributes) }

func (r *module) createJob(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var spec *starlark.Dict
	if err := starlark.UnpackArgs("createJob", args, kwargs, "spec", &spec); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	var sparkJob v2pb.SparkJob
	if err := utils.AsGo(spec, &sparkJob); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var createRes v2pb.CreateSparkJobResponse
	if err := workflow.ExecuteActivity(ctx, spark.Activities.CreateSparkJob, v2pb.CreateSparkJobRequest{
		SparkJob: &sparkJob,
	}).Get(ctx, &createRes); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	job := *createRes.SparkJob

	var res starlark.Value
	if err := utils.AsStar(job, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return res, nil
}

// waits till a specific condition is meet (blocking call) .
//
//	sensor_job(job, timeout_seconds=0, poll_seconds=10, assert_condition_type="succeeded") -> job
//
//	  job: a spark job crd in json format
//	  timeout_seconds: int: job is expected to finish within the given time
//	  poll_seconds: int: job status poll interval
//
//	  return: dict: job status
func (r *module) sensorJob(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var _job *starlark.Dict
	timeout := int64(utils.CadenceLongTimeout.Seconds())
	poll := _defaultPollSeconds
	var assertConditionType string = utils.SucceededCondition

	if err := starlark.UnpackArgs("sensor_job", args, kwargs,
		"job", &_job,
		"assert_condition_type?", &assertConditionType,
	); err != nil {
		logger.Error(_errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}
	var sparkJob v2pb.SparkJob
	if err := utils.AsGo(_job, &sparkJob); err != nil {
		logger.Error(_errorReasonConvertJob, ext.ZapError(err)...)
		return nil, err
	}

	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	var status v2pb.SparkJobStatus

	getSparkJobRequest := v2pb.GetSparkJobRequest{
		Name:      sparkJob.Name,
		Namespace: sparkJob.Namespace,
	}
	var getSparkJobResponse v2pb.GetSparkJobResponse
	maxSensorTries := _maxJobSensorRetries
	for i := 0; i < maxSensorTries; i++ {
		if err := workflow.ExecuteActivity(sensorCtx, spark.Activities.SensorSparkJob, getSparkJobRequest, &status).Get(ctx, &getSparkJobResponse); err != nil {
			if cadence.IsCanceledError(err) {
				// killing spark job in cadence once workflow is cancelled
				ctx, _ = workflow.NewDisconnectedContext(ctx)
				terminateRequest := spark.TerminateSparkJobRequest{
					Name:      sparkJob.Name,
					Namespace: sparkJob.Namespace,
					Type:      v2pb.TERMINATION_TYPE_FAILED,
					Reason:    _reasonForCancel,
				}
				var terminateResponse v2pb.UpdateSparkJobResponse
				if terminateErr := workflow.ExecuteActivity(ctx, spark.Activities.TerminateSparkJob, terminateRequest).Get(ctx, &terminateResponse); terminateErr != nil {
					logger.Error(_errorReasonTermninateJob, ext.ZapError(terminateErr)...)
					return nil, terminateErr
				}
				var res starlark.Value
				if convertErr := utils.AsStar(terminateResponse.SparkJob, &res); convertErr != nil {
					logger.Error(_errorReasonConvertJob, ext.ZapError(err)...)
					return nil, convertErr
				}
				return res, nil
			}
			logger.Error(_errorReasonSensorJob, ext.ZapError(err)...)
			continue
		}
		status = getSparkJobResponse.SparkJob.Status
		// we will break as long as succeeded condition has been set
		succeeded := spark.GetCondition(utils.SucceededCondition, status.GetStatusConditions())
		if succeeded != nil && succeeded.Status != apipb.CONDITION_STATUS_UNKNOWN {
			break
		}
		// check the condition to assert
		assertCondition := spark.GetCondition(assertConditionType, status.GetStatusConditions())
		if assertCondition != nil && assertCondition.Status != apipb.CONDITION_STATUS_UNKNOWN {
			// if the assertConditionType is RUNNING, we also want to ensure that the log url is also generated.
			if assertCondition.Type == utils.SparkAppRunningCondition && status.JobUrl != "" {
				break
			}
		}
	}

	var sparkJobValue starlark.Value
	if err := utils.AsStar(getSparkJobResponse.SparkJob, &sparkJobValue); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}
	return sparkJobValue, nil
}

func getRunningConditionType(reciever starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.SparkAppRunningCondition), nil
}

func getSucceededConditionType(reciever starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.SucceededCondition), nil
}

func getKilledConditionType(reciever starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.KilledCondition), nil
}
