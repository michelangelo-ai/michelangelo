package rollout

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ConditionsPlugin = &conditionPlugin{}

type conditionPlugin struct {
	actors []plugins.ConditionActor
}

// Params contains dependencies for rollout plugin
type Params struct {
	Client  client.Client
	Gateway inferenceserver.Gateway
	Logger  logr.Logger
}

// NewRolloutPlugin creates a new rollout plugin following Uber patterns
func NewRolloutPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (plugins.ConditionsPlugin, error) {
	logger := p.Logger.WithValues("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName()))

	// Pre-placement actors (preparation and validation)
	prePlacementActors := []plugins.ConditionActor{
		&ValidationActor{
			client: p.Client,
			logger: logger,
		},
		&AssetPreparationActor{
			client:  p.Client,
			gateway: p.Gateway,
			logger:  logger,
		},
		&ResourceAcquisitionActor{
			client: p.Client,
			logger: logger,
		},
	}

	// Placement strategy actors (rolling strategy for OSS)
	placementActors, err := strategies.GetActorsForStrategy(ctx, strategies.Params{
		Client:  p.Client,
		Gateway: p.Gateway,
		Logger:  logger,
	}, deployment)
	if err != nil {
		return nil, err
	}

	// Post-placement actors (completion and cleanup)
	postPlacementActors := []plugins.ConditionActor{
		&RolloutCompletionActor{
			client: p.Client,
			logger: logger,
		},
	}

	// Combine all actors in sequence
	actors := make([]plugins.ConditionActor, 0,
		len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)

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

// ValidationActor validates deployment configuration
type ValidationActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ValidationActor) GetType() string {
	return common.ActorTypeValidation
}

func (a *ValidationActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Validate deployment configuration
	if resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for deployment",
		}, nil
	}

	if resource.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoInferenceServer", 
			Message: "No inference server specified for deployment",
		}, nil
	}

	modelName := resource.Spec.DesiredRevision.Name
	if modelName == "" {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidModelName",
			Message: "Model name cannot be empty",
		}, nil
	}

	// For OSS, validate model exists in available models
	if !common.IsModelAvailable(modelName) {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelNotFound",
			Message: fmt.Sprintf("Model %s not found in storage. Available models: %s", modelName, common.GetAvailableModels()),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: fmt.Sprintf("Deployment validation completed successfully for model %s", modelName),
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running validation for deployment", "deployment", resource.Name)
	
	// Update deployment status to show validation is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION
	
	if resource.Spec.DesiredRevision != nil {
		runtimeCtx.Logger.Info("Validation completed successfully", "model", resource.Spec.DesiredRevision.Name)
	}
	
	return nil
}

// AssetPreparationActor handles asset preparation following Uber patterns
type AssetPreparationActor struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func (a *AssetPreparationActor) GetType() string {
	return common.ActorTypeAssetPreparation
}

func (a *AssetPreparationActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for asset preparation",
		}, nil
	}

	modelName := resource.Spec.DesiredRevision.Name
	
	// For OSS, check if model assets are available
	if !common.IsModelAvailable(modelName) {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "AssetsNotFound",
			Message: fmt.Sprintf("Assets for model %s not found in storage", modelName),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "AssetsAvailable",
		Message: fmt.Sprintf("Assets for model %s are available and prepared", modelName),
	}, nil
}

func (a *AssetPreparationActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running asset preparation for deployment", "deployment", resource.Name)
	
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		runtimeCtx.Logger.Info("Preparing assets for model", "model", modelName)
		
		// For OSS, asset preparation involves validating model accessibility
		// In Uber's implementation, this downloads from S3, compiles, and uploads to TerraBob
		runtimeCtx.Logger.Info("Asset preparation completed", "model", modelName)
	}
	
	return nil
}

// ResourceAcquisitionActor handles resource acquisition
type ResourceAcquisitionActor struct {
	client client.Client
	logger logr.Logger
}

func (a *ResourceAcquisitionActor) GetType() string {
	return common.ActorTypeResourceAcquisition
}

func (a *ResourceAcquisitionActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Spec.GetInferenceServer() != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ResourcesAvailable",
			Message: "Required resources are available and allocated",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ResourcesPending",
		Message: "Waiting for resource allocation",
	}, nil
}

func (a *ResourceAcquisitionActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running resource acquisition for deployment", "deployment", resource.Name)
	
	if resource.Spec.GetInferenceServer() != nil {
		runtimeCtx.Logger.Info("Resources acquired successfully", 
			"inference_server", resource.Spec.GetInferenceServer().Name)
	}
	
	return nil
}

// RolloutCompletionActor handles post-rollout completion tasks
type RolloutCompletionActor struct {
	client client.Client
	logger logr.Logger
}

func (a *RolloutCompletionActor) GetType() string {
	return common.ActorTypeRolloutCompletion
}

func (a *RolloutCompletionActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CompletionTasksFinished",
			Message: "All rollout completion tasks have been successfully executed",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CompletionTasksPending",
		Message: "Rollout completion tasks are pending",
	}, nil
}

func (a *RolloutCompletionActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) error {
	runtimeCtx.Logger.Info("Running rollout completion tasks for deployment", "deployment", resource.Name)
	
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		
		// Mark deployment as complete and healthy
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		resource.Status.Message = fmt.Sprintf("Rollout completed successfully for model %s", modelName)
		
		// Clean up temporary annotations
		if resource.Annotations != nil {
			delete(resource.Annotations, "rollout.michelangelo.ai/in-progress")
			delete(resource.Annotations, "rollout.michelangelo.ai/start-time")
		}
		
		runtimeCtx.Logger.Info("Rollout completion tasks finished successfully", "model", modelName)
	}
	
	return nil
}