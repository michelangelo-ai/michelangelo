package apihook

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RegisterTriggerRunAPIHook registers the API hook that defaults the
// environment label on TriggerRun creation, stamps the owning Pipeline as
// the controller ownerReference, and stamps the owning Pipeline's type as
// the michelangelo/SourcePipelineType label. The Pipeline-related stamps
// are best-effort: if the owning Pipeline can't be resolved, creation
// proceeds without them. defaultEnv is the operator-configured default
// (api.UnspecifiedEnvironment is used instead when the operator has
// configured none), mirroring RegisterPipelineRunAPIHook.
func RegisterTriggerRunAPIHook(logger *zap.Logger, apiHandler api.Handler, scheme *runtime.Scheme, defaultEnv string) {
	v2.RegisterTriggerRunAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		scheme:     scheme,
		defaultEnv: defaultEnv,
	})
}

type apiHook struct {
	v2.NoopTriggerRunAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	scheme     *runtime.Scheme
	defaultEnv string
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateTriggerRunRequest) error {
	setIfAbsent(request.TriggerRun, api.EnvironmentLabel, a.defaultEnvironment())

	// TODO(https://github.com/michelangelo-ai/michelangelo/issues/2155): a
	// production-environment / branch-protection restriction on TriggerRun
	// create is still being designed and is not implemented here.

	pipelineRef := request.TriggerRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	namespace := pipelineRef.GetNamespace()
	if namespace == "" {
		namespace = request.TriggerRun.GetNamespace()
	}

	pipeline := &v2.Pipeline{}
	if err := a.apiHandler.Get(ctx, namespace, pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
		if utils.IsNotFoundError(err) {
			return nil
		}
		a.logger.Warn("BeforeCreate: failed to resolve owning Pipeline for ownerRef",
			zap.String("pipeline", pipelineRef.GetName()), zap.Error(err))
		return nil
	}

	if pipelineType := pipeline.Spec.GetType(); pipelineType != v2.PIPELINE_TYPE_INVALID {
		api.StampSourcePipelineTypeLabelOnCreate(request.TriggerRun, pipelineType.String())
	}

	return cascadedelete.StampOwnerRefOnCreate(ctx, a.logger, a.scheme, request.TriggerRun, pipeline)
}

// defaultEnvironment returns the configured default, or
// api.UnspecifiedEnvironment when the operator has configured none —
// mirrors pipelinerun/apihook.go's identically-named method.
func (a apiHook) defaultEnvironment() string {
	if a.defaultEnv == "" {
		return api.UnspecifiedEnvironment
	}
	return a.defaultEnv
}

func setIfAbsent(run *v2.TriggerRun, key, value string) {
	if run.ObjectMeta.Labels == nil {
		run.ObjectMeta.Labels = map[string]string{}
	}
	if _, ok := run.ObjectMeta.Labels[key]; !ok {
		run.ObjectMeta.Labels[key] = value
	}
}
