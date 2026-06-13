package pipeline

import (
	"context"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/revision"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// PipelineRevisionHandler dispatches Revision lifecycle events for Pipeline resources.
type PipelineRevisionHandler struct {
	logger *zap.Logger
}

// NewPipelineRevisionHandler constructs a PipelineRevisionHandler.
func NewPipelineRevisionHandler(logger *zap.Logger) *PipelineRevisionHandler {
	return &PipelineRevisionHandler{logger: logger}
}

func (h *PipelineRevisionHandler) TypeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: pipelineAPIVersion,
		Kind:       "Pipeline",
	}
}

func (h *PipelineRevisionHandler) Reconcile(ctx context.Context, rev *v2pb.Revision) error {
	h.logger.Info("pipeline revision handler called", zap.String("revision", rev.Name))
	return nil
}

var _ revision.Handler = (*PipelineRevisionHandler)(nil)
