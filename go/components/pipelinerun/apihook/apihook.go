package apihook

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	pbtypes "github.com/gogo/protobuf/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// pipelineKind is the Revision.Spec.BaseType.Kind value for Pipeline snapshots.
// Revisions also snapshot other resource kinds (models, deployments), which are
// not valid targets for a PipelineRun.
const pipelineKind = "Pipeline"

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
	// Fetch the live Pipeline once when a pipeline ref is present. Used both to
	// optionally pin status.latestRevision and to stamp ownerRef / notifications.
	pipeline, err := a.getReferencedPipeline(ctx, request)
	if err != nil {
		return err
	}

	// Resolve a revision into an inline PipelineSpec before ownerRef stamping, so
	// stamping and notification inheritance observe any backfilled Spec.Pipeline.
	//
	// Priority:
	//  1. Explicit Spec.Revision — must resolve; failure rejects the create.
	//  2. Else Pipeline.Status.LatestRevision — pin that snapshot when present.
	//     If the Revision CR is missing, fall back to the live Pipeline path.
	if request.PipelineRun.Spec.GetRevision().GetName() != "" {
		if err = a.resolveRevision(ctx, request); err != nil {
			a.logger.Error("BeforeCreate: failed to resolve pinned revision",
				zap.String("revision", request.PipelineRun.Spec.GetRevision().GetName()),
				zap.String("pipelinerun", request.PipelineRun.GetName()),
				zap.Error(err))
			return err
		}
	} else if err = a.tryResolveLatestRevision(ctx, request, pipeline); err != nil {
		a.logger.Error("BeforeCreate: failed to resolve pipeline status.latestRevision",
			zap.String("pipelinerun", request.PipelineRun.GetName()),
			zap.Error(err))
		return err
	}

	// resolveRevision may have backfilled Spec.Pipeline from the revision's base
	// resource; fetch now if we did not already have the live Pipeline.
	if pipeline == nil {
		pipeline, err = a.getReferencedPipeline(ctx, request)
		if err != nil {
			return err
		}
	}
	if pipeline == nil {
		a.logger.Info("BeforeCreate: no pipeline reference, skipping ownerRef/notifications")
		return nil
	}

	if len(request.PipelineRun.Spec.Notifications) == 0 && len(pipeline.Spec.Notifications) > 0 {
		request.PipelineRun.Spec.Notifications = pipeline.Spec.Notifications
		a.logger.Info("BeforeCreate: copied notifications from Pipeline to PipelineRun",
			zap.Int("count", len(pipeline.Spec.Notifications)))
	}

	return cascadedelete.StampOwnerRefOnCreate(ctx, a.logger, a.scheme, request.PipelineRun, pipeline)
}

// getReferencedPipeline loads Spec.Pipeline when set. Any Get failure (including
// not-found) returns (nil, nil): latestRevision pinning then no-ops (live
// fallback), and ownerRef/notification stamping is skipped.
func (a apiHook) getReferencedPipeline(ctx context.Context, request *v2.CreatePipelineRunRequest) (*v2.Pipeline, error) {
	pipelineRef := request.PipelineRun.Spec.GetPipeline()
	if pipelineRef.GetName() == "" {
		return nil, nil
	}

	namespace := resourceNamespace(pipelineRef, request.PipelineRun.GetNamespace())
	a.logger.Info("BeforeCreate: fetching pipeline",
		zap.String("pipeline", pipelineRef.GetName()),
		zap.String("namespace", namespace),
		zap.String("pipelinerun", request.PipelineRun.GetName()))

	pipeline := &v2.Pipeline{}
	if err := a.apiHandler.Get(ctx, namespace, pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
		if utils.IsNotFoundError(err) {
			a.logger.Info("BeforeCreate: pipeline not found",
				zap.String("pipeline", pipelineRef.GetName()),
				zap.String("namespace", namespace))
			return nil, nil
		}
		// Soft for ownerRef/notifications: a transient Get failure should not
		// block PipelineRun create. Callers that require the Pipeline (latest
		// revision pinning) treat nil as "no pin".
		a.logger.Warn("BeforeCreate: failed to get Pipeline",
			zap.String("pipeline", pipelineRef.GetName()),
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, nil
	}
	return pipeline, nil
}

// tryResolveLatestRevision sets Spec.Revision from pipeline.Status.LatestRevision
// and resolves it. Missing latestRevision (nil pipeline, unset pointer, empty
// name, or Revision CR not found) leaves the request unchanged so execution
// falls back to the live Pipeline. Other resolution errors reject the create.
func (a apiHook) tryResolveLatestRevision(ctx context.Context, request *v2.CreatePipelineRunRequest, pipeline *v2.Pipeline) error {
	if request.PipelineRun.Spec.GetPipelineSpec() != nil {
		// Dev-run / already-inlined spec wins; do not override with latestRevision.
		return nil
	}
	if pipeline == nil {
		return nil
	}

	latest := pipeline.Status.GetLatestRevision()
	if latest.GetName() == "" {
		a.logger.Info("BeforeCreate: pipeline has no status.latestRevision; falling back to live Pipeline",
			zap.String("pipeline", pipeline.Name),
			zap.String("namespace", pipeline.Namespace))
		return nil
	}

	revisionNamespace := resourceNamespace(latest, pipeline.Namespace)
	request.PipelineRun.Spec.Revision = &apipb.ResourceIdentifier{
		Name:      latest.GetName(),
		Namespace: revisionNamespace,
	}

	if err := a.resolveRevision(ctx, request); err != nil {
		// Clear the tentative pin so SourcePipelineActor still sees a plain
		// live-Pipeline run when the Revision CR is gone.
		request.PipelineRun.Spec.Revision = nil
		request.PipelineRun.Spec.PipelineSpec = nil
		if utils.IsNotFoundError(err) {
			a.logger.Info("BeforeCreate: status.latestRevision CR not found; falling back to live Pipeline",
				zap.String("revision", latest.GetName()),
				zap.String("namespace", revisionNamespace),
				zap.String("pipeline", pipeline.Name),
				zap.Error(err))
			return nil
		}
		return err
	}

	a.logger.Info("BeforeCreate: pinned status.latestRevision into inline pipeline spec",
		zap.String("revision", latest.GetName()),
		zap.String("namespace", revisionNamespace),
		zap.String("pipeline", pipeline.Name),
		zap.String("pipelinerun", request.PipelineRun.GetName()))
	return nil
}

// resourceNamespace returns ref.Namespace, or fallback when the ref omits it.
func resourceNamespace(ref *apipb.ResourceIdentifier, fallback string) string {
	if ns := ref.GetNamespace(); ns != "" {
		return ns
	}
	return fallback
}

// resolveRevision loads the Revision CR pinned by Spec.Revision and normalises
// the request into the same shape as a dev-run: the snapshotted Pipeline's spec
// is assigned to Spec.PipelineSpec, so the execution path (SourcePipelineActor)
// needs no revision-aware logic and never reads the live, mutable Pipeline.
//
// It also:
//   - backfills Spec.Pipeline from the revision's base resource when the caller
//     supplied only a revision, so ownerRef stamping and notification
//     inheritance keep working;
//   - carries the snapshot's annotations (e.g. the uniflow image ID) onto the
//     PipelineRun, since the dev-run path builds the synthesized Pipeline's
//     annotations from the PipelineRun rather than a live Pipeline.
//
// Any resolution failure is returned as an error so the create is rejected.
func (a apiHook) resolveRevision(ctx context.Context, request *v2.CreatePipelineRunRequest) error {
	revisionRef := request.PipelineRun.Spec.GetRevision()

	if request.PipelineRun.Spec.GetPipelineSpec() != nil {
		return fmt.Errorf("pipeline run %q sets both spec.revision (%s) and an inline spec.pipelineSpec; supply exactly one",
			request.PipelineRun.GetName(), revisionRef.GetName())
	}

	namespace := resourceNamespace(revisionRef, request.PipelineRun.GetNamespace())

	rev := &v2.Revision{}
	if err := a.apiHandler.Get(ctx, namespace, revisionRef.GetName(), &metav1.GetOptions{}, rev); err != nil {
		// Returned unwrapped: the handler already reports namespace and name, and
		// callers classify it with utils.IsNotFoundError, which does not walk the
		// error chain.
		return err
	}

	// Revisions snapshot several resource kinds. Pinning a run to a non-Pipeline
	// revision is a caller bug and must be a loud error, not a fallback.
	kind := ""
	if baseType := rev.Spec.GetBaseType(); baseType != nil {
		kind = baseType.Kind
	}
	if kind != pipelineKind {
		return fmt.Errorf("revision %s/%s snapshots kind %q, not %q; it cannot be used for a pipeline run",
			namespace, revisionRef.GetName(), kind, pipelineKind)
	}

	if rev.Spec.GetContent() == nil {
		return fmt.Errorf("revision %s/%s has no snapshotted content", namespace, revisionRef.GetName())
	}

	pipeline := &v2.Pipeline{}
	if err := pbtypes.UnmarshalAny(rev.Spec.Content, pipeline); err != nil {
		return fmt.Errorf("unmarshal snapshotted pipeline from revision %s/%s: %w", namespace, revisionRef.GetName(), err)
	}

	request.PipelineRun.Spec.PipelineSpec = &pipeline.Spec

	if request.PipelineRun.Spec.GetPipeline().GetName() == "" {
		request.PipelineRun.Spec.Pipeline = rev.Spec.GetBaseResource()
	}

	for k, v := range pipeline.GetAnnotations() {
		if request.PipelineRun.Annotations == nil {
			request.PipelineRun.Annotations = map[string]string{}
		}
		// PipelineRun-supplied annotations win over the snapshot's.
		if _, ok := request.PipelineRun.Annotations[k]; !ok {
			request.PipelineRun.Annotations[k] = v
		}
	}

	a.logger.Info("BeforeCreate: resolved pinned revision into inline pipeline spec",
		zap.String("revision", revisionRef.GetName()),
		zap.String("namespace", namespace),
		zap.String("pipelinerun", request.PipelineRun.GetName()))

	return nil
}
