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

// RegisterPipelineRunAPIHook registers the API hook that stamps the owning
// Pipeline as the controller ownerReference on PipelineRuns at creation, so a run
// is never GC-eligible-but-unprotected. Resolving the owning Pipeline is
// kind-specific and happens here; the shared stamping body lives in
// cascadedelete.StampOwnerRefOnCreate.
func RegisterPipelineRunAPIHook(logger *zap.Logger, apiHandler api.Handler, scheme *runtime.Scheme) {
	v2.RegisterPipelineRunAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		scheme:     scheme,
	})
}

type apiHook struct {
	v2.NoopPipelineRunAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	scheme     *runtime.Scheme
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreatePipelineRunRequest) error {
	pipelineRef := request.PipelineRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	namespace := pipelineRef.GetNamespace()
	if namespace == "" {
		namespace = request.PipelineRun.GetNamespace()
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

	return cascadedelete.StampOwnerRefOnCreate(ctx, a.logger, a.scheme, request.PipelineRun, pipeline)
}
