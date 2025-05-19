package deployment

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.starlark.net/starlark"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
)

const pluginID = "deployment"

var poll int64 = 10

var Plugin = &plugin{}

type plugin struct{}

var _ service.IPlugin = (*plugin)(nil)

func (r *plugin) ID() string {
	return pluginID
}
func (r *plugin) Create(_ service.RunInfo) starlark.Value {
	return newModule()
}
func (r *plugin) Register(_ worker.Registry) {}

type module struct {
	attributes map[string]starlark.Value
}

func (m *module) String() string                        { return pluginID }
func (m *module) Type() string                          { return pluginID }
func (m *module) Freeze()                               {}
func (m *module) Truth() starlark.Bool                  { return true }
func (m *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (m *module) Attr(n string) (starlark.Value, error) { return m.attributes[n], nil }
func (m *module) AttrNames() []string                   { return ext.SortedKeys(m.attributes) }
func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return star.Decode(b, out)
}
func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"deploy": starlark.NewBuiltin("deploy", m.deploy).BindReceiver(m),
	}
	return m
}

func (m *module) deploy(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace string
	var name string
	var modelName string
	if err := starlark.UnpackArgs("deploy", args, kwargs,
		"namespace", &namespace,
		"name", &name,
		"model_name", &modelName,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var res v2pb.GetDeploymentResponse
	request := deployment.SensorRolloutRequest{
		Name:      name,
		Namespace: namespace,
		ModelName: modelName,
	}

	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	if err := workflow.ExecuteActivity(sensorCtx, deployment.Activities.SensorRollout, request).Get(ctx, &res); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	dep := res.GetDeployment()
	var ret starlark.Value
	if err := AsStar(dep, &ret); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	return ret, nil
}
