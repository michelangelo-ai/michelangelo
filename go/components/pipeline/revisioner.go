package pipeline

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	pipelineAPIVersion = "michelangelo.api/v2"
	pipelineKind       = "Pipeline"
)

// Revisioner produces a Revision CR for the given Pipeline. The default
// implementation writes the CR via the standard k8s API handler; a NoOp
// implementation skips creation entirely. Operators or consumers wire the
// desired implementation via Fx.
type Revisioner interface {
	Snapshot(ctx context.Context, pipeline *v2pb.Pipeline) error
}

// NewDefaultRevisioner returns the default Revisioner: writes Revision CRs
// via the standard k8s API handler, treating AlreadyExists as a no-op
// (revisions are immutable).
func NewDefaultRevisioner(handler api.Handler, logger *zap.Logger) Revisioner {
	return &defaultRevisioner{handler: handler, logger: logger}
}

type defaultRevisioner struct {
	handler api.Handler
	logger  *zap.Logger
}

// Snapshot creates a Revision CR for the pipeline. No-op if the pipeline has
// no commit info; revisions are identified by git ref. Returns nil if a
// revision for the same identity already exists.
func (r *defaultRevisioner) Snapshot(ctx context.Context, pipeline *v2pb.Pipeline) error {
	if pipeline.Spec.Commit == nil {
		r.logger.Info("skipping revision snapshot: pipeline has no commit info",
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return nil
	}

	rev, err := revision.NewCR(revision.UpsertRevisionParams{
		RevisionName: formatRevisionName(pipeline),
		RevisionID:   pipeline.Spec.Commit.GitRef,
		Content:      pipeline,
		Owner:        pipeline.Spec.GetOwner(),
		BaseType: &metav1.TypeMeta{
			Kind:       pipelineKind,
			APIVersion: pipelineAPIVersion,
		},
		BaseResource: &apipb.ResourceIdentifier{
			Name:      pipeline.Name,
			Namespace: pipeline.Namespace,
		},
		Source:      revision.SourceGit,
		GitCommit:   pipeline.Spec.Commit,
		Annotations: pipeline.Annotations,
	})
	if err != nil {
		return fmt.Errorf("build pipeline revision: %w", err)
	}

	if err := r.handler.Create(ctx, rev, nil); err != nil {
		if revision.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

// NewNoOpRevisioner returns a Revisioner that does nothing. Useful when
// revisioning is disabled for the pipeline controller.
func NewNoOpRevisioner() Revisioner { return noOpRevisioner{} }

type noOpRevisioner struct{}

func (noOpRevisioner) Snapshot(_ context.Context, _ *v2pb.Pipeline) error { return nil }
