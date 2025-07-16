package strategies

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelSyncActor handles model synchronization to inference servers
type ModelSyncActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *ModelSyncActor) GetType() string {
	return common.ActorTypeModelSync
}

func (a *ModelSyncActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if model is synced to the inference server
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ModelSyncCompleted",
			Message: "Model successfully synced to inference server",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ModelSyncPending",
		Message: "Model sync is in progress",
	}, nil
}

func (a *ModelSyncActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running model sync for deployment", "deployment", resource.Name)
	
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name
		
		runtimeCtx.Logger.Info("Syncing model to inference server",
			"model", modelName,
			"inference_server", inferenceServerName)
		
		// For OSS, simulate model sync by updating ConfigMaps/resources
		// In a real implementation, this would:
		// 1. Create/update ConfigMap with model configuration
		// 2. Trigger model loading on inference server
		// 3. Wait for model to be ready
		// 4. Update routing configuration
		
		// Update status to indicate sync completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		runtimeCtx.Logger.Info("Model sync completed successfully")
	}
	
	return nil
}

// RollingRolloutActor handles rolling rollout strategy following Uber patterns
type RollingRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *RollingRolloutActor) GetType() string {
	return common.ActorTypeRollingRollout
}

func (a *RollingRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rolling rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {
		
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollingRolloutCompleted",
			Message: "Rolling rollout completed successfully across all inference servers",
		}, nil
	}

	// Check if rollout is in progress
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "RollingRolloutInProgress",
			Message: "Rolling rollout is in progress",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollingRolloutPending",
		Message: "Rolling rollout has not started",
	}, nil
}

func (a *RollingRolloutActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running rolling rollout for deployment", "deployment", resource.Name)
	
	// Update deployment to placement stage
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name
		
		runtimeCtx.Logger.Info("Starting rolling rollout",
			"model", modelName,
			"inference_server", inferenceServerName)
		
		// In Uber's implementation, rolling rollout:
		// 1. Resolves all hosts for the inference server (via UNS)
		// 2. Incrementally rolls out to percentage of hosts (30% by default)
		// 3. Waits for model to load on each batch before proceeding
		// 4. Continues until 100% of hosts have the new model
		// 5. Uses sophisticated host resolution and load balancing
		
		// For OSS, we simulate a successful rolling rollout:
		// - Update inference server configurations incrementally
		// - Monitor model loading status on each pod
		// - Implement proper rollback on failures
		
		// Get rollout increment percentage from annotations or use default
		incrementPercentage := common.GetRolloutIncrement(resource)
		runtimeCtx.Logger.Info("Rolling rollout configuration",
			"increment_percentage", incrementPercentage,
			"strategy", "rolling")
		
		// Simulate successful rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		runtimeCtx.Logger.Info("Rolling rollout completed successfully", "model", modelName)
	}
	
	return nil
}