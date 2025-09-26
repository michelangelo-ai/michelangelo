package rollback

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ConditionsPlugin = &conditionPlugin{}

type conditionPlugin struct {
	actors []plugins.ConditionActor
}

// Params contains dependencies for rollback plugin
type Params struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  logr.Logger
}

// NewRollbackPlugin creates a new rollback plugin following Uber patterns
func NewRollbackPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (plugins.ConditionsPlugin, error) {
	logger := p.Logger.WithValues("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName()))

	actors := []plugins.ConditionActor{
		&RollbackActor{
			client:  p.Client,
			gateway: p.Gateway,
			logger:  logger,
		},
	}

	return &conditionPlugin{
		actors: actors,
	}, nil
}

// GetActors returns all actors for this plugin
func (p *conditionPlugin) GetActors() []plugins.ConditionActor {
	return p.actors
}

// GetConditions gets the conditions for a deployment
func (p *conditionPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition puts a condition for a deployment
func (p *conditionPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// RollbackActor handles rollback operations following Uber patterns
type RollbackActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *RollbackActor) GetType() string {
	return common.ActorTypeRollback
}

func (a *RollbackActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rollback is complete when we restore to the previous revision
	if resource.Status.CurrentRevision != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollbackCompleted",
			Message: "Rollback completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollbackInProgress",
		Message: "Rollback in progress",
	}, nil
}

func (a *RollbackActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running rollback for deployment", "deployment", resource.Name)
	
	// Update deployment status to indicate rollback is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
	resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
	
	if resource.Status.CurrentRevision != nil {
		// In Uber's implementation, rollback involves:
		// 1. Identify the previous known good revision
		// 2. Validate rollback target is available and healthy
		// 3. Update UCS cache to rollback model references
		// 4. Execute reverse rolling deployment to previous revision
		// 5. Monitor rollback progress and validate success
		// 6. Update MES records and clean up failed rollout artifacts
		
		// Store the failed revision for reference
		failedRevision := resource.Spec.DesiredRevision
		
		// For OSS, rollback means restoring the previous revision
		resource.Spec.DesiredRevision = resource.Status.CurrentRevision
		
		// Update status to reflect rollback completion
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		
		runtimeCtx.Logger.Info("Rolled back to previous revision", 
			"from", failedRevision.Name, 
			"to", resource.Status.CurrentRevision.Name)
	} else {
		runtimeCtx.Logger.Info("No previous revision available for rollback")
	}

	return nil
}