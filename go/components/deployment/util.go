package deployment

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/revision"
)

// UpsertDeploymentRevision upserts a deployment revision (simplified no-op version)
func UpsertDeploymentRevision(ctx context.Context, deployment *v2pb.Deployment, revisionManager revision.Manager) error {
	// In simplified version, revision management is disabled
	return nil
}

// removeConditionsForDeployment removes conditions that are no longer relevant
func removeConditionsForDeployment(deployment *v2pb.Deployment, conditionPlugin interface{}) {
	// In the simplified version, we don't need to remove specific conditions
	// Just clear all conditions when moving to terminal states
}
