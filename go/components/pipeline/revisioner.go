package pipeline

import (
	"context"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	pipelineAPIVersion = "michelangelo.api/v2"
	pipelineKind       = "Pipeline"
)

// Revisioner produces a Revision CR for the given Pipeline. The default
// implementation delegates to revision.Manager; a NoOp implementation skips
// creation entirely. Operators or consumers wire the desired implementation
// via Fx.
type Revisioner interface {
	Snapshot(ctx context.Context, pipeline *v2pb.Pipeline) error
}

// NewDefaultRevisioner returns the default Revisioner backed by revision.Manager.
func NewDefaultRevisioner(mgr revision.Manager, logger *zap.Logger) Revisioner {
	return &defaultRevisioner{manager: mgr, logger: logger}
}

type defaultRevisioner struct {
	manager revision.Manager
	logger  *zap.Logger
}

func (r *defaultRevisioner) Snapshot(ctx context.Context, pipeline *v2pb.Pipeline) error {
	if pipeline.Spec.Commit == nil {
		r.logger.Info("skipping revision snapshot: pipeline has no commit info",
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return nil
	}

	_, err := r.manager.UpsertRevision(ctx, revision.UpsertRevisionParams{
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
	return err
}

// NewNoOpRevisioner returns a Revisioner that does nothing. Useful when
// revisioning is disabled for the pipeline controller.
func NewNoOpRevisioner() Revisioner { return noOpRevisioner{} }

type noOpRevisioner struct{}

func (noOpRevisioner) Snapshot(_ context.Context, _ *v2pb.Pipeline) error { return nil }
