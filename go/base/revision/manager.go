package revision

import (
	"context"
	"fmt"

	"go.uber.org/yarpc"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type revisionManager struct {
	handler api.Handler
	logger  *zap.Logger
}

// NewManager creates a Manager backed by the given API handler.
func NewManager(handler api.Handler, logger *zap.Logger) Manager {
	return &revisionManager{handler: handler, logger: logger}
}

func (m *revisionManager) UpsertRevision(ctx context.Context, rev client.Object, opts UpsertOpts, _ ...yarpc.CallOption) (bool, error) {
	namespace := rev.GetNamespace()
	name := rev.GetName()
	logger := m.logger.With(
		zap.String("revision_name", name),
		zap.String("namespace", namespace),
	)

	existing := rev.DeepCopyObject().(client.Object)
	err := m.handler.Get(ctx, namespace, name, &metav1.GetOptions{}, existing)
	if err != nil {
		if !apiutils.IsNotFoundError(err) {
			return false, fmt.Errorf("get existing revision: %w", err)
		}

		if opts.Immutable {
			apiutils.MarkImmutable(rev)
		}
		if createErr := m.handler.Create(ctx, rev, &metav1.CreateOptions{}); createErr != nil {
			return false, fmt.Errorf("create revision %s/%s: %w", namespace, name, createErr)
		}
		logger.Info("created revision")
		return true, nil
	}

	if apiutils.IsImmutable(existing) {
		if opts.Immutable {
			return false, nil
		}
		return false, fmt.Errorf("cannot update immutable revision %s to mutable", name)
	}

	rev.SetResourceVersion(existing.GetResourceVersion())
	if opts.Immutable {
		apiutils.MarkImmutable(rev)
	}
	if updateErr := m.handler.Update(ctx, rev, &metav1.UpdateOptions{}); updateErr != nil {
		return false, fmt.Errorf("update revision %s/%s: %w", namespace, name, updateErr)
	}
	logger.Info("updated revision")
	return true, nil
}
