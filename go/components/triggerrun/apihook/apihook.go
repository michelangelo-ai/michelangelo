package apihook

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RegisterTriggerRunAPIHook registers the API hook that stamps the owning
// Pipeline as the controller ownerReference on TriggerRuns at creation, and
// stamps the owning Pipeline's type as the michelangelo/SourcePipelineType
// label. Both are best-effort: if the owning Pipeline can't be resolved,
// creation proceeds without them.
func RegisterTriggerRunAPIHook(logger *zap.Logger, apiHandler api.Handler, scheme *runtime.Scheme) {
	v2.RegisterTriggerRunAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		scheme:     scheme,
	})
}

type apiHook struct {
	v2.NoopTriggerRunAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	scheme     *runtime.Scheme
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateTriggerRunRequest) error {
	if err := a.checkRevisionEnvironment(ctx, request); err != nil {
		return err
	}

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

// checkRevisionEnvironment enforces environment-label presence for a
// TriggerRun that pins a specific Revision. The check is skipped entirely
// when Spec.Revision is unset, since this rule only guards the
// revision-pinned creation path.
func (a apiHook) checkRevisionEnvironment(ctx context.Context, request *v2.CreateTriggerRunRequest) error {
	revisionRef := request.TriggerRun.Spec.GetRevision()
	if revisionRef == nil || revisionRef.GetName() == "" {
		return nil
	}

	envLabel := request.TriggerRun.GetLabels()[api.EnvironmentLabel]
	if envLabel == "" {
		return fmt.Errorf("environment label is not set; a trigger run pinned to a revision must have %s set", api.EnvironmentLabel)
	}

	// TODO(https://github.com/michelangelo-ai/michelangelo/issues/2155): add a
	// production-environment branch-protection check here — when envLabel is
	// "production", resolve the pinned Revision (via a.apiHandler.Get, the
	// same pattern as pipelinerun/apihook.go's resolveRevision) and reject
	// the create unless the revision's git branch is master/main. Deferred
	// pending that issue's design; presence enforcement above is unaffected.

	return nil
}
