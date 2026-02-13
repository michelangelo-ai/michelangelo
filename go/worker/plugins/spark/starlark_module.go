package spark

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.starlark.net/starlark"
)

// These are some error reasons
const (
	errorReasonUnpackArgs           = "UnpackArgsError"
	errorReasonConvertJob           = "ConvertSparkJobError"
	errorReasonConvertStarlarkValue = "ConvertStarlarkValueError"
	errorReasonSubmitJob            = "SubmitJobError"
	errorReasonSensorJob            = "SensorJobError"
	errorReasonTermninateJob        = "TerminateJobError"
)

const reasonForCancel = "Canceled by request"

// These are general const
const (
	defaultPollSeconds  = 10
	maxJobSensorRetries = 100
)

// TODO(#561): andrii: implement Spark starlark plugin here

var _ starlark.HasAttrs = (*module)(nil)
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
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var _job *starlark.Dict
	var timeout int64

	if err := starlark.UnpackArgs("create_job", args, kwargs,
		"job", &_job,
		"timeout_seconds?", &timeout,
	); err != nil {
		logger.Error(errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}
	if timeout == 0 {
		timeout = int64(utils.CadenceLongTimeout.Seconds())
	}

	var sparkJob v2pb.SparkJob
	if err := utils.AsGo(_job, &sparkJob); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	srp := utils.CadenceDefaultRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	createCtx := workflow.WithRetryPolicy(ctx, srp)

	var createRes spark.CreateSparkJobActivityResponse
	if err := workflow.ExecuteActivity(createCtx, spark.Activities.CreateSparkJob, v2pb.CreateSparkJobRequest{
		SparkJob: &sparkJob,
	}).Get(ctx, &createRes); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	enhancedResponse := map[string]interface{}{
		"sparkJob":   createRes.SparkJob,
		"activityId": createRes.ActivityID,
	}

	var res starlark.Value
	if err := utils.AsStar(enhancedResponse, &res); err != nil {
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
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var _job *starlark.Dict
	timeout := int64(utils.CadenceLongTimeout.Seconds())
	poll := defaultPollSeconds
	var assertConditionType string = utils.SucceededCondition

	if err := starlark.UnpackArgs("sensor_job", args, kwargs,
		"job", &_job,
		"assert_condition_type?", &assertConditionType,
	); err != nil {
		logger.Error(errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}
	var sparkJob v2pb.SparkJob
	if err := utils.AsGo(_job, &sparkJob); err != nil {
		logger.Error(errorReasonConvertJob, ext.ZapError(err)...)
		return nil, err
	}

	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	getSparkJobRequest := v2pb.GetSparkJobRequest{
		Name:      sparkJob.Name,
		Namespace: sparkJob.Namespace,
	}
	var getSparkJobResponse spark.SensorSparkJobResponse
	maxSensorTries := maxJobSensorRetries
	for i := 0; i < maxSensorTries; i++ {
		if err := workflow.ExecuteActivity(sensorCtx, spark.Activities.SensorSparkJob, getSparkJobRequest).Get(ctx, &getSparkJobResponse); err != nil {
			if workflow.IsCanceledError(ctx, err) {
				// killing spark job in cadence once workflow is cancelled
				ctx, _ = workflow.NewDisconnectedContext(ctx)
				terminateRequest := spark.TerminateSparkJobRequest{
					Name:      sparkJob.Name,
					Namespace: sparkJob.Namespace,
					Type:      v2pb.TERMINATION_TYPE_FAILED,
					Reason:    reasonForCancel,
				}
				var terminateResponse v2pb.UpdateSparkJobResponse
				if terminateErr := workflow.ExecuteActivity(ctx, spark.Activities.TerminateSparkJob, terminateRequest).Get(ctx, &terminateResponse); terminateErr != nil {
					logger.Error(errorReasonTermninateJob, ext.ZapError(terminateErr)...)
					return nil, terminateErr
				}
				var res starlark.Value
				if convertErr := utils.AsStar(terminateResponse.SparkJob, &res); convertErr != nil {
					logger.Error(errorReasonConvertJob, ext.ZapError(err)...)
					return nil, convertErr
				}
				return res, nil
			}
			logger.Error(errorReasonSensorJob, ext.ZapError(err)...)
			continue
		}
		// we will break as long as succeeded condition has been set
		if getSparkJobResponse.Terminal {
			break
		}
	}

	var sparkJobValue starlark.Value
	if err := utils.AsStar(getSparkJobResponse.SparkJob, &sparkJobValue); err != nil {
		logger.Error(errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}
	return sparkJobValue, nil
}

func getRunningConditionType(receiver starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.SparkAppRunningCondition), nil
}

func getSucceededConditionType(receiver starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.SucceededCondition), nil
}

func getKilledConditionType(receiver starlark.Value) (starlark.Value, error) {
	return starlark.String(utils.KilledCondition), nil
}
