package ray

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/star"
	jsoniter "github.com/json-iterator/go"
	"go.starlark.net/starlark"
	"go.uber.org/cadence"
	"go.uber.org/cadence/workflow"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TODO: andrii: implement Ray starlark plugin here

var _ starlark.HasAttrs = (*module)(nil)
var timeout int64 = 0
var poll int64 = 10

type module struct {
	attributes map[string]starlark.Value
}

const CadenceLongTimeout = time.Hour * 24 * 365 * 10 // 10 years, practically - no timeout

var CadenceDefaultNonRetriableErrorReasons = []string{
	"cadenceInternal:Panic",                  // panics
	"cadenceInternal:Generic",                // cadence converter errors (similar to invalid-argument)
	"400",                                    // bad-request https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/400
	"401",                                    // unauthorized
	"403",                                    // forbidden
	"404",                                    // not-found
	"405",                                    // method-not-allowed
	"502",                                    // bad-gateway
	yarpcerrors.CodeCancelled.String(),       // client error
	yarpcerrors.CodeNotFound.String(),        // client error
	yarpcerrors.CodeAlreadyExists.String(),   // client error
	yarpcerrors.CodeInvalidArgument.String(), // client error
	yarpcerrors.CodeUnauthenticated.String(), // client error
	yarpcerrors.CodePermissionDenied.String(), // client error
	yarpcerrors.CodeUnimplemented.String(),    // client error
	yarpcerrors.CodeDataLoss.String(),         // server error; unrecoverable data corruption
	yarpcerrors.CodeInternal.String(),         // server error; serious error, like panic
}

var CadenceDefaultRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 15,
	BackoffCoefficient:       1,
	ExpirationInterval:       time.Minute * 5,
	NonRetriableErrorReasons: CadenceDefaultNonRetriableErrorReasons,
	MaximumAttempts:          1,
}

var CadenceDefaultSensorRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 10,
	BackoffCoefficient:       1,
	ExpirationInterval:       CadenceLongTimeout,
	NonRetriableErrorReasons: CadenceDefaultNonRetriableErrorReasons,
	MaximumAttempts:          100,
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_cluster":    starlark.NewBuiltin("create_cluster", m.createCluster).BindReceiver(m),
		"terminate_cluster": starlark.NewBuiltin("terminate_cluster", m.terminateCluster).BindReceiver(m),
		"create_job":        starlark.NewBuiltin("create_job", m.createJob).BindReceiver(m),
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
func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return star.Decode(b, out)
}
func AsGo(source starlark.Value, out any) error {
	b, err := star.Encode(source)
	if err != nil {
		return err
	}
	return jsoniter.Unmarshal(b, out)
}

func (r *module) createCluster(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var spec *starlark.Dict
	if err := starlark.UnpackArgs("create_cluster", args, kwargs, "spec", &spec); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	var cluster v2pb.RayCluster
	if err := AsGo(spec, &cluster); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var response v2pb.CreateRayClusterResponse
	if err := workflow.ExecuteActivity(ctx, ray.Activities.CreateRayCluster, cluster).Get(ctx, &response); err != nil {
		logger.Error("error", zap.Error(err))
		return nil, err
	}

	cluster = *response.RayCluster

	srp := CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	sensorRequest := v2pb.GetRayClusterRequest{
		Name:       cluster.Name,
		Namespace:  cluster.Namespace,
		GetOptions: &metav1.GetOptions{},
	}
	var sensorResponse ray.SensorRayClusterReadinessResponse
	var printJobURL = true
	for sensorResponse.Ready == false {
		if err := workflow.ExecuteActivity(sensorCtx, ray.Activities.SensorRayClusterReadiness, sensorRequest).Get(sensorCtx, &sensorResponse); err != nil {
			logger.Error("builtin-error", ext.ZapError(err)...)
			reason := err.Error()
			if cadence.IsCanceledError(err) {
				ctx, _ = workflow.NewDisconnectedContext(ctx)
				reason = "Canceled"
			}
			if err = workflow.ExecuteActivity(ctx, ray.Activities.TerminateCluster, ray.TerminateClusterRequest{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Type:      v2pb.TERMINATION_TYPE_FAILED.String(),
				Reason:    reason,
			}).Get(ctx, nil); err != nil {
				logger.Error("builtin-error", ext.ZapError(err)...)
			}
			return nil, err
		}
		if sensorResponse.JobURL != "" {
			// Sensor activity has returned JobURL. Disable ReturnJobURL early-return flag for the next sensor calls, if any.
			if printJobURL {
				t.Print(t, "ray | create cluster: url="+sensorResponse.JobURL)
				printJobURL = false
			}
		}
	}
	cluster = *sensorResponse.RayCluster

	if cluster.Status.State == v2pb.RAY_CLUSTER_STATE_FAILED || cluster.Status.State == v2pb.RAY_CLUSTER_STATE_TERMINATED || cluster.Status.State == v2pb.RAY_CLUSTER_STATE_UNKNOWN {
		// TODO: [ray] send termination signal?
		err := cadence.NewCustomError(
			yarpcerrors.CodeInternal.String(),
			fmt.Sprintf("Ray cluster is not ready: %s/%s", cluster.Namespace, cluster.Name),
		)
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	sensorCluster := sensorResponse.RayCluster
	var res starlark.Value
	if err := AsStar(sensorCluster, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return res, nil
}

func (r *module) createJob(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var entrypoint string
	var rayClusterNamespace string
	var rayClusterName string

	if err := starlark.UnpackArgs("create_job", args, kwargs,
		"entrypoint", &entrypoint,
		"ray_job_namespace?", &rayClusterNamespace,
		"ray_job_name?", &rayClusterName,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Start submit a ray job here
	rayJob := v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("uf-rj-%v-", rayClusterName),
			Namespace:    fmt.Sprintf("%v", rayClusterNamespace),
		},
		Spec: v2pb.RayJobSpec{
			User:       nil,
			Entrypoint: entrypoint,
			JobId:      "",
			Cluster: &apipb.ResourceIdentifier{
				Namespace: rayClusterNamespace,
				Name:      rayClusterName,
			},
		},
	}
	var createRes v2pb.CreateRayJobResponse
	if err := workflow.ExecuteActivity(ctx, ray.Activities.CreateRayJob, v2pb.CreateRayJobRequest{
		RayJob: &rayJob,
	}).Get(ctx, &createRes); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	rayJob = *createRes.RayJob

	var sensorRes ray.SensorRayJobResponse
	srp := CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	if err := workflow.ExecuteActivity(sensorCtx, ray.Activities.SensorRayJob, v2pb.GetRayJobRequest{
		Name:       createRes.RayJob.Name,
		Namespace:  createRes.RayJob.Namespace,
		GetOptions: &metav1.GetOptions{},
	}).Get(sensorCtx, &sensorRes); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	job := sensorRes.RayJob
	var res starlark.Value
	if err := AsStar(job, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return res, nil
}

func (r *module) terminateCluster(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var name string
	var namespce string
	var reason string
	var terminateTypeStr string

	if err := starlark.UnpackArgs("terminate_job", args, kwargs,
		"name", &name,
		"namespce", &namespce,
		"reason", &reason,
		"terminateType", &terminateTypeStr,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var res v2pb.UpdateRayClusterResponse
	srp := CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	if err := workflow.ExecuteActivity(sensorCtx, ray.Activities.TerminateCluster, ray.TerminateClusterRequest{
		Name:      name,
		Namespace: namespce,
		Type:      terminateTypeStr,
		Reason:    reason,
	}).Get(sensorCtx, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	cluster := res.RayCluster
	if cluster.Status.State == v2pb.RAY_CLUSTER_STATE_TERMINATED {
		return starlark.Bool(true), nil
	}

	return starlark.Bool(false), nil
}
