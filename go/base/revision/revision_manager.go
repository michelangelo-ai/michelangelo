package revision

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type revisionManager struct {
	handler api.Handler
	logger  *zap.Logger
}

// NewManager creates a Manager backed by the given API handler.
func NewManager(handler api.Handler, logger *zap.Logger) Manager {
	return &revisionManager{handler: handler, logger: logger}
}

func (m *revisionManager) UpsertRevision(ctx context.Context, params UpsertRevisionParams) (bool, error) {
	logger := m.logger.With(
		zap.String("revision_name", params.RevisionName),
		zap.String("base_type_kind", params.BaseType.Kind),
	)

	existing := &v2pb.Revision{}
	err := m.handler.Get(ctx, params.BaseResource.Namespace, params.RevisionName, &metav1.GetOptions{}, existing)
	if err != nil {
		if !apiutils.IsNotFoundError(err) {
			return false, fmt.Errorf("get existing revision: %w", err)
		}

		// Not found — create.
		rev, buildErr := NewCR(params)
		if buildErr != nil {
			return false, buildErr
		}
		if params.Immutable {
			apiutils.MarkImmutable(rev)
		}
		if createErr := m.handler.Create(ctx, rev, &metav1.CreateOptions{}); createErr != nil {
			return false, fmt.Errorf("create revision %s/%s: %w", params.BaseResource.Namespace, params.RevisionName, createErr)
		}
		logger.Info("created revision")
		return true, nil
	}

	// Exists — check immutability.
	if apiutils.IsImmutable(existing) {
		if params.Immutable {
			return false, nil
		}
		return false, fmt.Errorf("cannot update immutable revision %s to mutable", params.RevisionName)
	}

	// Mutable existing revision — update content + labels.
	rev, buildErr := NewCR(params)
	if buildErr != nil {
		return false, buildErr
	}
	existing.Spec.Content = rev.Spec.Content
	for k, v := range params.Labels {
		if existing.Labels == nil {
			existing.Labels = map[string]string{}
		}
		existing.Labels[k] = v
	}
	if params.Immutable {
		apiutils.MarkImmutable(existing)
	}
	if updateErr := m.handler.Update(ctx, existing, &metav1.UpdateOptions{}); updateErr != nil {
		return false, fmt.Errorf("update revision %s/%s: %w", params.BaseResource.Namespace, params.RevisionName, updateErr)
	}
	logger.Info("updated revision")
	return false, nil
}

func (m *revisionManager) GetRevision(ctx context.Context, namespace, name string) (*v2pb.Revision, error) {
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("namespace and name must be non-empty")
	}
	rev := &v2pb.Revision{}
	if err := m.handler.Get(ctx, namespace, name, &metav1.GetOptions{}, rev); err != nil {
		return nil, err
	}
	return rev, nil
}

func (m *revisionManager) FetchRevisionID(ctx context.Context, namespace, name string) (string, error) {
	rev, err := m.GetRevision(ctx, namespace, name)
	if err != nil {
		return "", err
	}
	return rev.Spec.GetRevisionId(), nil
}

func (m *revisionManager) DeleteAllRevisions(ctx context.Context, namespace, name, kind string) error {
	listOpts := &metav1.ListOptions{
		LabelSelector: LabelSelectorFor(namespace, name, kind),
	}
	revList := &v2pb.RevisionList{}
	if err := m.handler.List(ctx, namespace, listOpts, nil, revList); err != nil {
		return fmt.Errorf("list revisions for %s/%s: %w", namespace, name, err)
	}
	for i := range revList.Items {
		if err := m.handler.Delete(ctx, &revList.Items[i], &metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete revision %s: %w", revList.Items[i].Name, err)
		}
	}
	return nil
}
