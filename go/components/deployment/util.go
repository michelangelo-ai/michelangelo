package deployment

import (
	"context"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// UpsertDeploymentRevision upserts a deployment revision (simplified no-op version)
func UpsertDeploymentRevision(ctx context.Context, deployment *v2pb.Deployment, revisionManager revision.Manager) error {
	// In simplified version, revision management is disabled
	return nil
}

// removeConditionsForDeployment removes conditions that are no longer relevant
func removeConditionsForDeployment(
	deployment *v2pb.Deployment,
	plugin conditionInterfaces.Plugin[*v2pb.Deployment],
) {
	if plugin == nil {
		return
	}
	newCondition := []*api.Condition{}
	for _, condition := range deployment.Status.Conditions {
		for _, actor := range plugin.GetActors() {
			if condition.GetType() == actor.GetType() {
				newCondition = append(newCondition, condition)
			}
		}
	}
	deployment.Status.Conditions = newCondition
}
