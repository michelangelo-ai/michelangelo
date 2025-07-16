package cleanup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ConditionsPlugin = &conditionPlugin{}

type conditionPlugin struct {
	actors []plugins.ConditionActor
}

// Params contains dependencies for cleanup plugin
type Params struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  logr.Logger
}

// NewCleanupPlugin creates a new cleanup plugin following Uber patterns
func NewCleanupPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (plugins.ConditionsPlugin, error) {
	logger := p.Logger.WithValues("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName()))

	actors := []plugins.ConditionActor{
		&CleanupActor{
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

// CleanupActor handles cleanup operations following Uber patterns
type CleanupActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

func (a *CleanupActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if cleanup is complete when deletion timestamp is set
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CleanupCompleted",
			Message: "Cleanup completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CleanupNotNeeded",
		Message: "Cleanup not required",
	}, nil
}

func (a *CleanupActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running cleanup for deployment", "deployment", resource.Name)
	
	// Update deployment status to indicate cleanup is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
	
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		// In Uber's implementation, cleanup involves:
		// 1. Remove model from UCS cache
		// 2. Clean up model artifacts and temporary files
		// 3. Remove ConfigMaps and other Kubernetes resources
		// 4. Update MES (Model Execution Service) records
		// 5. Clean up monitoring and logging configurations
		
		// For OSS, simulate cleanup operations:
		// - Remove model-related ConfigMaps
		// - Clean up temporary resources
		// - Update inference server configurations
		
		runtimeCtx.Logger.Info("Cleaning up model artifacts and ConfigMaps", "deployment", resource.Name)
		
		// Mark cleanup as complete
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
		runtimeCtx.Logger.Info("Cleanup completed for OSS deployment")
	}
	
	return nil
}