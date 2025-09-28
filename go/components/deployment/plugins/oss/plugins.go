package oss

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Simple plugin implementations for OSS

// RolloutPlugin handles rollout operations
type RolloutPlugin struct {
	client            client.Client
	gateway           gateways.Gateway
	blobstore         *blobstore.BlobStore
	dynamicClient     dynamic.Interface
	configMapProvider *gateways.ConfigMapProvider
	logger            logr.Logger
}

func NewRolloutPlugin(client client.Client, gateway gateways.Gateway, blobstore *blobstore.BlobStore, logger logr.Logger) conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &RolloutPlugin{
		client:        client,
		gateway:       gateway,
		blobstore:     blobstore,
		dynamicClient: nil, // Not provided in legacy constructor
		logger:        logger,
	}
}

func NewRolloutPluginWithDynamicClient(client client.Client, gateway gateways.Gateway, blobstore *blobstore.BlobStore, dynamicClient dynamic.Interface, logger logr.Logger) conditionInterfaces.Plugin[*v2pb.Deployment] {
	// Create ConfigMapProvider for deployment-level model sync following Uber's UCS cache pattern
	configMapProvider := gateways.NewConfigMapProvider(client, logger)

	return &RolloutPlugin{
		client:            client,
		gateway:           gateway,
		blobstore:         blobstore,
		dynamicClient:     dynamicClient,
		configMapProvider: configMapProvider,
		logger:            logger,
	}
}

func (p *RolloutPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	// Pre-placement actors (following Uber pattern)
	prePlacementActors := []plugins.ConditionActor{
		&ValidationActor{client: p.client, blobstore: p.blobstore, logger: p.logger},
		&AssetPreparationActor{client: p.client, gateway: p.gateway, logger: p.logger},
		&ResourceAcquisitionActor{client: p.client, logger: p.logger},
	}

	// Placement strategy actors (OSS rolling strategy with ConfigMapProvider for UCS cache pattern)
	placementActors := []plugins.ConditionActor{
		&ModelSyncActor{
			client:            p.client,
			gateway:           p.gateway,
			dynamicClient:     p.dynamicClient,
			configMapProvider: p.configMapProvider,
			logger:            p.logger,
		},
		&RollingRolloutActor{client: p.client, gateway: p.gateway, logger: p.logger},
	}

	// Post-placement actors (with ConfigMapProvider for cleanup)
	postPlacementActors := []plugins.ConditionActor{
		&RolloutCompletionActor{
			client:            p.client,
			gateway:           p.gateway,
			configMapProvider: p.configMapProvider,
			logger:            p.logger,
		},
	}

	// Combine all actors in sequence
	actors := make([]plugins.ConditionActor, 0, len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)

	// Convert to the expected interface
	result := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], len(actors))
	for i, actor := range actors {
		result[i] = &ActorWrapper{actor: actor}
	}

	return result
}

func (p *RolloutPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	// The v2pb.Deployment.Status.Conditions is already []*apipb.Condition
	return resource.Status.Conditions
}

func (p *RolloutPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	// Find existing condition and update it, or append new one
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	// If not found, append new condition
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// HandleCleanup handles cleanup when a deployment is being deleted, including ConfigMaps and other resources
func (p *RolloutPlugin) HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("RolloutPlugin: Starting cleanup for deployment", "deployment", deployment.Name)

	if deployment.Spec.GetInferenceServer() == nil {
		logger.Info("No inference server specified, skipping ConfigMap cleanup")
		return nil
	}

	inferenceServerName := deployment.Spec.GetInferenceServer().Name

	// UCS CLEANUP PATTERN: Remove deployment from deployment registry (following Uber's asset lifecycle management)
	if p.configMapProvider != nil {
		logger.Info("UCS cleanup: Removing deployment from deployment registry", "deployment", deployment.Name, "inferenceServer", inferenceServerName)

		// Remove deployment from registry following UCS pattern
		if err := p.configMapProvider.RemoveDeploymentFromRegistry(ctx, inferenceServerName, deployment.Namespace, deployment.Name); err != nil {
			logger.Error(err, "Failed to remove deployment from registry during cleanup")
			// Don't fail cleanup for registry errors, but log them
		}

		// UCS FLUSH PATTERN: After removing deployment, flush merged state to trigger model cleanup
		logger.Info("UCS cleanup: Flushing merged state to clean up unused models", "inferenceServer", inferenceServerName)
		if err := p.configMapProvider.FlushMergedStateToModelConfig(ctx, inferenceServerName, deployment.Namespace); err != nil {
			logger.Error(err, "Failed to flush merged state during cleanup")
			// Don't fail cleanup for flush errors, but log them
		}

		logger.Info("UCS cleanup pattern completed successfully", "deployment", deployment.Name)
	}

	// Additional cleanup via gateway if available
	if p.gateway != nil {
		logger.Info("Performing additional gateway cleanup", "deployment", deployment.Name)

		// Clean up any deployment-specific routes or configurations
		// Note: HTTPRoutes are typically managed separately by Kubernetes garbage collection
		// but we can do explicit cleanup if needed
	}

	logger.Info("RolloutPlugin: Cleanup completed successfully", "deployment", deployment.Name)
	return nil
}

// CleanupPlugin handles cleanup operations
type CleanupPlugin struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func NewCleanupPlugin(client client.Client, gateway gateways.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &CleanupPlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *CleanupPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&CleanupActor{client: p.client, logger: p.logger},
	}
}

func (p *CleanupPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *CleanupPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// HandleCleanup handles cleanup when a deployment is being deleted
func (p *CleanupPlugin) HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("CleanupPlugin: Starting cleanup for deployment", "deployment", deployment.Name)

	// CleanupPlugin is focused on general cleanup tasks
	// ConfigMap cleanup is handled by RolloutPlugin which has the ConfigMapProvider

	logger.Info("CleanupPlugin: General cleanup completed", "deployment", deployment.Name)
	return nil
}

// RollbackPlugin handles rollback operations
type RollbackPlugin struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func NewRollbackPlugin(client client.Client, gateway gateways.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &RollbackPlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *RollbackPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&RollbackActor{client: p.client, logger: p.logger},
	}
}

func (p *RollbackPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *RollbackPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// HandleCleanup handles cleanup when a deployment is being deleted
func (p *RollbackPlugin) HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("RollbackPlugin: Starting cleanup for deployment", "deployment", deployment.Name)

	// RollbackPlugin focuses on rollback operations
	// Main ConfigMap cleanup is handled by RolloutPlugin

	logger.Info("RollbackPlugin: Cleanup completed", "deployment", deployment.Name)
	return nil
}

// SteadyStatePlugin handles steady state monitoring
type SteadyStatePlugin struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func NewSteadyStatePlugin(client client.Client, gateway gateways.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &SteadyStatePlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *SteadyStatePlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&SteadyStateActor{client: p.client, logger: p.logger},
	}
}

func (p *SteadyStatePlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *SteadyStatePlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// HandleCleanup handles cleanup when a deployment is being deleted
func (p *SteadyStatePlugin) HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("SteadyStatePlugin: Starting cleanup for deployment", "deployment", deployment.Name)

	// SteadyStatePlugin focuses on monitoring
	// Main ConfigMap cleanup is handled by RolloutPlugin

	logger.Info("SteadyStatePlugin: Cleanup completed", "deployment", deployment.Name)
	return nil
}
