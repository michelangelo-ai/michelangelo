// Package apihook registers the Model API hook used to default and inherit
// the environment label on Models at create/update time. See
// go/components/pipelinerun/apihook and go/components/triggerrun/apihook for
// the sibling packages this one follows the shape of.
//
// This hook implements api.EnvironmentLabel
// defaulting/inheritance. It deliberately does not implement
// description-length validation, pipeline-type label copy, owner/LDAP
// validation, or revision/pipeline-name label copy — those have no obvious
// OSS-generic equivalent and are out of scope here.
//
// BeforeUpdate re-derives the label from Spec.SourcePipelineRun on every
// update, the same as BeforeCreate: the source PipelineRun's label is the
// source of truth, so a manual edit to a Model's label is overwritten again
// on the next update if a source is still set. This is intentional, not an
// oversight.
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
	return a.applyEnvironmentLabel(ctx, request.Model)
}

func (a apiHook) BeforeUpdate(ctx context.Context, request *v2.UpdateModelRequest) error {
	return a.applyEnvironmentLabel(ctx, request.Model)
}

// applyEnvironmentLabel sets api.EnvironmentLabel to the configured default
// (or api.UnspecifiedEnvironment when unconfigured) if absent, then
// overrides it with the source PipelineRun's value when one is set,
// resolvable, and itself carries the label — default first, then
// overwrite-if-present-on-source.
func (a apiHook) applyEnvironmentLabel(ctx context.Context, model *v2.Model) error {
	setIfAbsent(model, api.EnvironmentLabel, a.defaultEnvironment())

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
			a.logger.Warn("applyEnvironmentLabel: source PipelineRun not found, keeping default environment label",
				zap.String("pipelineRun", srcRef.GetName()))
			return nil
		}
		// Non-not-found errors (e.g. transient API-server failure) are logged
		// and swallowed: this is a best-effort label-inheritance lookup and
		// should never block Model creation, matching
		// pipelinerun/apihook.go's precedent of treating Get failures as
		// non-fatal for optional enrichment.
		a.logger.Warn("applyEnvironmentLabel: failed to resolve source PipelineRun for environment-label inheritance",
			zap.String("pipelineRun", srcRef.GetName()), zap.Error(err))
		return nil
	}

	if val, ok := src.ObjectMeta.Labels[api.EnvironmentLabel]; ok {
		setLabel(model, api.EnvironmentLabel, val)
	}
	return nil
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
