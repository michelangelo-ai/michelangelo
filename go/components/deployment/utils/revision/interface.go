package revision

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
)

// Manager handles revision operations
type Manager interface {
	UpsertRevision(ctx context.Context, deployment *types.Deployment) error
	DeleteAllRevisions(ctx context.Context, namespace, name, resourceType string) error
}

// NoOpManager is a revision manager that does nothing
type NoOpManager struct{}

// NewNoOpManager creates a new no-op revision manager
func NewNoOpManager() Manager {
	return &NoOpManager{}
}

// UpsertRevision does nothing and returns success
func (m *NoOpManager) UpsertRevision(ctx context.Context, deployment *types.Deployment) error {
	return nil
}

// DeleteAllRevisions does nothing and returns success
func (m *NoOpManager) DeleteAllRevisions(ctx context.Context, namespace, name, resourceType string) error {
	return nil
}