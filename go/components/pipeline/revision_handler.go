package pipeline

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	revisionctrl "github.com/michelangelo-ai/michelangelo/go/components/revision"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RevisionHandler advances the status.state of Pipeline-backed Revisions from
// the state of the referenced base Pipeline. It is dispatched by the Revision
// controller for revisions whose Spec.BaseType is michelangelo.api/v2 Pipeline.
type RevisionHandler struct {
	handler api.Handler
	logger  *zap.Logger
}

var _ revisionctrl.Handler = (*RevisionHandler)(nil)

// NewRevisionHandler constructs a RevisionHandler backed by the given API handler.
func NewRevisionHandler(handler api.Handler, logger *zap.Logger) *RevisionHandler {
	return &RevisionHandler{
		handler: handler,
		logger:  logger.With(zap.String("revision-handler", pipelineKind)),
	}
}

// newRevisionHandler builds a RevisionHandler from the manager's client for FX wiring.
func newRevisionHandler(mgr manager.Manager, factory apiHandler.Factory, logger *zap.Logger) (*RevisionHandler, error) {
	handler, err := factory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return nil, err
	}
	return NewRevisionHandler(handler, logger), nil
}

// TypeMeta implements revision.Handler.
func (h *RevisionHandler) TypeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{APIVersion: pipelineAPIVersion, Kind: pipelineKind}
}

// Reconcile implements revision.Handler. It sets rev.Status.State from the base
// Pipeline; the Revision controller persists the change and marks the revision
// immutable once it reaches READY or ERROR.
func (h *RevisionHandler) Reconcile(ctx context.Context, rev *v2pb.Revision) (ctrl.Result, error) {
	base := rev.Spec.GetBaseResource()
	if base == nil || strings.TrimSpace(base.Name) == "" {
		rev.Status.State = v2pb.REVISION_STATE_ERROR
		return ctrl.Result{}, nil
	}

	namespace := strings.TrimSpace(base.Namespace)
	if namespace == "" {
		namespace = rev.Namespace
	}
	name := strings.TrimSpace(base.Name)

	pipeline := &v2pb.Pipeline{}
	if err := h.handler.Get(ctx, namespace, name, &metav1.GetOptions{}, pipeline); err != nil {
		if utils.IsNotFoundError(err) {
			rev.Status.State = v2pb.REVISION_STATE_ERROR
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get base pipeline %s/%s: %w", namespace, name, err)
	}

	state, result := revisionStateFromPipeline(pipeline.Status.GetState())
	if state != rev.Status.State {
		h.logger.Info("revision status updated",
			zap.String("revision", rev.Namespace+"/"+rev.Name),
			zap.String("from", rev.Status.State.String()),
			zap.String("to", state.String()),
		)
	}
	rev.Status.State = state
	return result, nil
}

// revisionStateFromPipeline maps a Pipeline state to the corresponding Revision
// state, requeueing while the pipeline is still in a non-terminal state.
func revisionStateFromPipeline(state v2pb.PipelineState) (v2pb.RevisionState, ctrl.Result) {
	switch state {
	case v2pb.PIPELINE_STATE_READY:
		return v2pb.REVISION_STATE_READY, ctrl.Result{}
	case v2pb.PIPELINE_STATE_ERROR, v2pb.PIPELINE_STATE_INVALID:
		return v2pb.REVISION_STATE_ERROR, ctrl.Result{}
	case v2pb.PIPELINE_STATE_CREATED:
		return v2pb.REVISION_STATE_CREATED, ctrl.Result{RequeueAfter: reconcileInterval}
	default:
		return v2pb.REVISION_STATE_BUILDING, ctrl.Result{RequeueAfter: reconcileInterval}
	}
}
