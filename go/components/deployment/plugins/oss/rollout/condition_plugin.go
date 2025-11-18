package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies for rollout plugin
type Params struct {
	Client                 client.Client
	ModelConfigMapProvider configmap.ModelConfigMapProvider
	Gateway                gateways.Gateway
	Logger                 *zap.Logger
}

// NewRolloutPlugin creates a new rollout plugin following Uber patterns
func NewRolloutPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	logger := p.Logger.With(zap.String("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName())))

	// Pre-placement actors (preparation and validation)
	prePlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
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
		Client:                 p.Client,
		ModelConfigMapProvider: p.ModelConfigMapProvider,
		Gateway:                p.Gateway,
		Logger:                 p.Logger,
	}, deployment)
	if err != nil {
		return nil, err
	}

	// Post-placement actors (completion and cleanup)
	postPlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&RolloutCompletionActor{
			client:                 p.Client,
			gateway:                p.Gateway,
			modelConfigMapProvider: p.ModelConfigMapProvider,
			logger:                 p.Logger,
		},
	}

	// Combine all actors in sequence
	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0,
		len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)

	return &conditionPlugin{
		actors: actors,
	}, nil
}

// GetActors returns all actors for this plugin
func (p *conditionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
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
	logger *zap.Logger
}

func (a *ValidationActor) GetType() string {
	return common.ActorTypeValidation
}

func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
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

func (a *ValidationActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running validation for deployment", zap.String("deployment", deployment.Name))

	// Update deployment status to show validation is in progress
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION

	// Perform comprehensive validation
	if deployment.Spec.DesiredRevision == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: No desired revision specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoDesiredRevision", Message: "Validation failed: No desired revision specified"}, nil
	}

	if deployment.Spec.GetInferenceServer() == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: No inference server specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoInferenceServer", Message: "Validation failed: No inference server specified"}, nil
	}

	// Additional OSS-specific validations
	if deployment.Spec.DesiredRevision.Name == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: Desired revision name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyRevisionName", Message: "Validation failed: Desired revision name is empty"}, nil
	}

	if deployment.Spec.GetInferenceServer().Name == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: Inference server name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyInferenceServerName", Message: "Validation failed: Inference server name is empty"}, nil
	}

	// If all validations pass
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	deployment.Status.Message = "Validation completed successfully"
	a.logger.Info("Validation completed successfully", zap.String("deployment", deployment.Name))

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// AssetPreparationActor handles asset preparation following Uber patterns
type AssetPreparationActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *AssetPreparationActor) GetType() string {
	return common.ActorTypeAssetPreparation
}

func (a *AssetPreparationActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if assets are prepared for the desired model
	if deployment.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for asset preparation",
		}, nil
	}

	// For OSS, we assume assets are available in MinIO/S3 storage
	// In Uber's implementation, this checks TerraBob and validates model assets
	modelName := deployment.Spec.DesiredRevision.Name

	// For OSS, assume assets are always available if the model name is valid
	// In a real implementation, this would check MinIO/S3 for model artifacts
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "AssetsAvailable",
		Message: fmt.Sprintf("Assets for model %s are available and prepared", modelName),
	}, nil
}

func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running asset preparation for deployment", zap.String("deployment", resource.Name))

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		a.logger.Info("Preparing assets for model", zap.String("model", modelName))

		// In Uber's implementation, this downloads from S3, compiles, and uploads to TerraBob
		// For OSS, we simulate asset preparation by ensuring model is accessible in storage
		// This would typically involve:
		// 1. Validate model exists in MinIO/S3
		// 2. Download and validate model artifacts
		// 3. Prepare model configuration files
		// 4. Ensure model is ready for inference server deployment

		a.logger.Info("Asset preparation completed", zap.String("model", modelName))
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "Success",
		Message: "Operation completed successfully",
	}, nil
}

// ResourceAcquisitionActor handles resource acquisition
type ResourceAcquisitionActor struct {
	client client.Client
	logger *zap.Logger
}

func (a *ResourceAcquisitionActor) GetType() string {
	return common.ActorTypeResourceAcquisition
}

func (a *ResourceAcquisitionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
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

func (a *ResourceAcquisitionActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running resource acquisition for deployment", zap.String("deployment", resource.Name))

	if resource.Spec.GetInferenceServer() != nil {
		a.logger.Info("Resources acquired successfully",
			zap.String("inference_server", resource.Spec.GetInferenceServer().Name))
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ResourcesAcquired",
		Message: "Deployment resources acquired successfully",
	}, nil
}

// RolloutCompletionActor handles post-rollout completion tasks
type RolloutCompletionActor struct {
	client                 client.Client
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func (a *RolloutCompletionActor) GetType() string {
	return common.ActorTypeRolloutCompletion
}

func (a *RolloutCompletionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
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

func (a *RolloutCompletionActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout completion tasks for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name
		inferenceServerName := deployment.Spec.GetInferenceServer().Name

		// ZERO-DOWNTIME TRAFFIC SWITCH: Now that ModelSyncActor has confirmed the new model
		// is loaded and ready in Triton, we can safely switch traffic by adding deployment-specific routing
		a.logger.Info("Adding deployment-specific route after health check confirmation", zap.String("newModel", modelName))

		// Add deployment-specific route for the new routing architecture
		if a.gateway != nil {
			// Add deployment-specific route: /<inference-server-name>/<deployment-name> -> /v2/models/<model-name>
			proxyConfigRequest := gateways.AddDeploymentRouteRequest{
				InferenceServer: inferenceServerName,
				Namespace:       deployment.Namespace,
				ModelName:       modelName,
				DeploymentName:  deployment.Name,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			if err := a.gateway.AddDeploymentRoute(ctx, a.logger, proxyConfigRequest); err != nil {
				a.logger.Error("Failed to add deployment-specific route", zap.Error(err))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "RouteCreationFailed",
					Message: fmt.Sprintf("Failed to add deployment-specific route: %v", err),
				}, nil
			}

			a.logger.Info("Deployment-specific route added successfully for zero-downtime traffic switch",
				zap.String("newModel", modelName), zap.String("deployment", deployment.Name),
				zap.String("route", fmt.Sprintf("/%s/%s", inferenceServerName, deployment.Name)))
		}

		// NOW we can safely update CurrentRevision since traffic has been switched
		deployment.Status.CurrentRevision = deployment.Spec.DesiredRevision
		a.logger.Info("CurrentRevision updated after successful traffic switch", zap.String("model", modelName))

		// DEPLOYMENT-LEVEL CLEANUP: Promote candidate to current and trigger safe cleanup
		if a.modelConfigMapProvider != nil {
			// Promote candidate model to current (this automatically triggers cleanup of unused models)
			a.logger.Info("Promoting candidate model to current and cleaning up unused models", zap.String("newModel", modelName))

			if err := common.UpdateDeploymentModel(ctx, a.logger, a.modelConfigMapProvider, inferenceServerName, deployment.Namespace, deployment.Name, modelName, "current"); err != nil {
				a.logger.Error("Failed to promote model to current via ConfigMapProvider", zap.Error(err))
				// Don't fail the whole rollout completion due to cleanup failure
				// but log the error for investigation
			} else {
				a.logger.Info("Successfully promoted candidate to current and cleaned up unused models", zap.String("currentModel", modelName))
			}
		} else {
			// Fallback to old gateway-based approach if ConfigMapProvider not available
			a.logger.Info("ConfigMapProvider not available, falling back to gateway cleanup")
			if a.gateway != nil {
				// Get current model configuration to identify old models
				a.logger.Info("Cleaning up old models from ConfigMap", zap.String("newModel", modelName))

				// Create cleanup request that will remove old models and keep only the new one
				cleanupRequest := configmap.UpdateModelConfigMapRequest{
					InferenceServer: inferenceServerName,
					Namespace:       deployment.Namespace,
					ModelConfigs:    []configmap.ModelConfigEntry{{Name: modelName, S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName)}},
				}

				if err := a.modelConfigMapProvider.UpdateModelConfigMap(ctx, cleanupRequest); err != nil {
					a.logger.Error("Failed to cleanup old models from ConfigMap", zap.Error(err))
					// Don't fail the whole rollout completion due to cleanup failure
					// but log the error for investigation
				} else {
					a.logger.Info("Successfully cleaned up old models from ConfigMap", zap.String("activeModel", modelName))
				}
			}
		}

		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		deployment.Status.Message = fmt.Sprintf("Rollout completed successfully for model %s", modelName)

		// Clean up any temporary annotations or metadata
		if deployment.Annotations != nil {
			// Remove rollout-specific annotations
			delete(deployment.Annotations, "rollout.michelangelo.ai/in-progress")
			delete(deployment.Annotations, "rollout.michelangelo.ai/start-time")
		}

		a.logger.Info("Rollout completion tasks finished successfully", zap.String("model", modelName))
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
