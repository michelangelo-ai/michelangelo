package spark

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TODO: andrii: implement Spark starlark plugin here

var _ starlark.HasAttrs = (*module)(nil)
var timeout int64 = 0
var poll int64 = 10

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_job": starlark.NewBuiltin("create_job", m.createJob).BindReceiver(m),
	}
	return m
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

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

	sparkJob = *createRes.SparkJob

	var sensorRes spark.SensorSparkJobResponse
	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	if err := workflow.ExecuteActivity(sensorCtx, spark.Activities.SensorSparkJob, v2pb.GetSparkJobRequest{
		Name:       createRes.SparkJob.Name,
		Namespace:  createRes.SparkJob.Namespace,
		GetOptions: &metav1.GetOptions{},
	}).Get(sensorCtx, &sensorRes); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	job := sensorRes.SparkJob
	var res starlark.Value
	if err := utils.AsStar(job, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return res, nil
}
