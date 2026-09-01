// Package apihook registers the Model API hook used to default and inherit
// labels and metadata on Models at create/update time. See
// go/components/pipelinerun/apihook and go/components/triggerrun/apihook for
// the sibling packages this one follows the shape of.
//
// BeforeUpdate re-derives inherited values from Spec.SourcePipelineRun on
// every update, the same as BeforeCreate: the source PipelineRun is the
// source of truth, so a manual edit to a Model's inherited label is
// overwritten again on the next update if a source is still set. This is
// intentional, not an oversight.
package apihook

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterModelAPIHook registers the API hook that defaults and inherits the
// environment label on Models at create/update time. defaultEnv is the
// operator-configured default (empty when unconfigured, in which case
// api.UnspecifiedEnvironment is used instead).
func RegisterModelAPIHook(logger *zap.Logger, apiHandler api.Handler, defaultEnv string) {
	v2.RegisterModelAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		defaultEnv: defaultEnv,
	})
}

type apiHook struct {
	v2.NoopModelAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	defaultEnv string
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateModelRequest) error {
	return a.applyFromSourcePipelineRun(ctx, request.Model)
}

func (a apiHook) BeforeUpdate(ctx context.Context, request *v2.UpdateModelRequest) error {
	return a.applyFromSourcePipelineRun(ctx, request.Model)
}

// applyFromSourcePipelineRun resolves Spec.SourcePipelineRun once, then
// applies all inherited labels from the resolved source. When the source
// is unset, unresolvable, or the Get fails, label defaults are still
// applied — the resolution is best-effort enrichment and never blocks
// Model creation.
func (a apiHook) applyFromSourcePipelineRun(ctx context.Context, model *v2.Model) error {
	setIfAbsent(model, api.EnvironmentLabel, a.defaultEnvironment())

	src := a.resolveSourcePipelineRun(ctx, model)
	if src == nil {
		return nil
	}

	a.applyEnvironmentLabel(model, src)
	return nil
}

// resolveSourcePipelineRun fetches the PipelineRun referenced by
// model.Spec.SourcePipelineRun. Returns nil when the reference is empty
// or the Get fails (not-found and transient errors alike are logged and
// swallowed).
func (a apiHook) resolveSourcePipelineRun(ctx context.Context, model *v2.Model) *v2.PipelineRun {
	srcRef := model.Spec.GetSourcePipelineRun()
	if srcRef.GetName() == "" {
		return nil
	}
	namespace := srcRef.GetNamespace()
	if namespace == "" {
		namespace = model.GetNamespace()
	}

	src := &v2.PipelineRun{}
	if err := a.apiHandler.Get(ctx, namespace, srcRef.GetName(), &metav1.GetOptions{}, src); err != nil {
		if utils.IsNotFoundError(err) {
			a.logger.Warn("resolveSourcePipelineRun: source PipelineRun not found, keeping defaults",
				zap.String("pipelineRun", srcRef.GetName()))
			return nil
		}
		a.logger.Warn("resolveSourcePipelineRun: failed to resolve source PipelineRun",
			zap.String("pipelineRun", srcRef.GetName()), zap.Error(err))
		return nil
	}
	return src
}

// applyEnvironmentLabel overrides api.EnvironmentLabel on the Model with
// the source PipelineRun's value when the source carries the label.
func (a apiHook) applyEnvironmentLabel(model *v2.Model, src *v2.PipelineRun) {
	if val, ok := src.ObjectMeta.Labels[api.EnvironmentLabel]; ok {
		setLabel(model, api.EnvironmentLabel, val)
	}
}

// defaultEnvironment returns the configured default, or
// api.UnspecifiedEnvironment when the operator has configured none.
func (a apiHook) defaultEnvironment() string {
	if a.defaultEnv == "" {
		return api.UnspecifiedEnvironment
	}
	return a.defaultEnv
}

func setIfAbsent(model *v2.Model, key, value string) {
	if model.ObjectMeta.Labels == nil {
		model.ObjectMeta.Labels = map[string]string{}
	}
	if _, ok := model.ObjectMeta.Labels[key]; !ok {
		model.ObjectMeta.Labels[key] = value
	}
}

func setLabel(model *v2.Model, key, value string) {
	if model.ObjectMeta.Labels == nil {
		model.ObjectMeta.Labels = map[string]string{}
	}
	model.ObjectMeta.Labels[key] = value
}
