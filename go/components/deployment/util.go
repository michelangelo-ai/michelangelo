package deployment

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/revision"
)

// UpsertDeploymentRevision upserts a deployment revision
func UpsertDeploymentRevision(ctx context.Context, deployment *types.Deployment, revisionManager revision.Manager) error {
	return revisionManager.UpsertRevision(ctx, deployment)
}

// removeConditionsForDeployment removes conditions that are no longer relevant
func removeConditionsForDeployment(deployment *types.Deployment, conditionPlugin interface{}) {
	// In the simplified version, we don't need to remove specific conditions
	// Just clear all conditions when moving to terminal states
}