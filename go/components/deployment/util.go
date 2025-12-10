package deployment

import (
	"context"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// UpsertDeploymentRevision creates or updates a deployment revision in the revision management system.
//
// In the current simplified implementation, revision management is disabled and this
// returns nil. Future implementations may add full revision tracking to a persistent store.
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
