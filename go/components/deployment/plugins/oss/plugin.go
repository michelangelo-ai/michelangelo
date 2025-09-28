package oss

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Subtype the subtype that the plugin represents.
const Subtype = "oss"

var _ plugins.Plugin = &Plugin{}

// Plugin is the OSS plugin implementation
type Plugin struct {
	client    client.Client
	gateway   gateways.Gateway
	blobstore *blobstore.BlobStore
	logger    logr.Logger
}

// NewPlugin creates a new instance of OSS plugin
func NewPlugin(client client.Client, gateway gateways.Gateway, blobstore *blobstore.BlobStore, logger logr.Logger) *Plugin {
	return &Plugin{
		client:    client,
		gateway:   gateway,
		blobstore: blobstore,
		logger:    logger,
	}
}

// GetRolloutPlugin returns the rollout plugin using the OSS rollout conditions plugin
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	// Return the OSS rollout conditions plugin
	return &OSSRolloutConditionsPlugin{
		client:     p.client,
		gateway:    p.gateway,
		blobstore:  p.blobstore,
		logger:     p.logger,
		deployment: deployment,
	}, nil
}

// GetRollbackPlugin returns the rollback plugin
func (p *Plugin) GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	// Return a simple conditions plugin using rollback actor
	return &OSSConditionsPlugin{
		actors: []plugins.ConditionActor{
			&RollbackActor{
				client: p.client,
				logger: p.logger,
			},
		},
	}
}

// GetCleanupPlugin returns the cleanup plugin
func (p *Plugin) GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	// Return a simple conditions plugin using cleanup actor
	return &OSSConditionsPlugin{
		actors: []plugins.ConditionActor{
			&CleanupActor{
				client: p.client,
				logger: p.logger,
			},
		},
	}
}

// GetSteadyStatePlugin returns the steady state plugin
func (p *Plugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	// Return a simple conditions plugin using steady state actor
	return &OSSConditionsPlugin{
		actors: []plugins.ConditionActor{
			&SteadyStateActor{
				client: p.client,
				logger: p.logger,
			},
		},
	}
}

// ParseStage goes through all the conditions and determines the current deployment stage
func (p *Plugin) ParseStage(deployment *v2pb.Deployment) v2pb.DeploymentStage {

	// Check if we need to trigger a new rollout despite having conditions
	// This happens when desired != candidate, which means a new rollout should start
	if deployment.Spec.DesiredRevision != nil && deployment.Status.CandidateRevision != nil {
		if deployment.Spec.DesiredRevision.Name != deployment.Status.CandidateRevision.Name {
			// New rollout needed - start from validation regardless of existing conditions
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		}
	}

	// If we have no conditions, start rollout process
	if len(deployment.Status.Conditions) == 0 {
		return v2pb.DEPLOYMENT_STAGE_VALIDATION
	}

	// Check for actual rollout completion conditions (not just steady state)
	hasRolloutComplete := false
	hasValidated := false
	hasModelSynced := false

	for _, cond := range deployment.Status.Conditions {
		switch cond.Type {
		case "RolloutComplete", "RolloutCompleted", "RollingRolloutComplete":
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				hasRolloutComplete = true
			}
		case "Validated", "TrafficRoutingConfigured":
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				hasValidated = true
			} else if cond.Status == apipb.CONDITION_STATUS_FALSE {
				return v2pb.DEPLOYMENT_STAGE_VALIDATION
			}
		case "ModelSynced":
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				hasModelSynced = true
			} else if cond.Status == apipb.CONDITION_STATUS_FALSE {
				return v2pb.DEPLOYMENT_STAGE_PLACEMENT
			}
		case "CleanupComplete":
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
			} else {
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
			}
		case "RollbackComplete":
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
			} else {
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
			}
		case "StateSteady":
			// StateSteady is not a rollout completion indicator - ignore it for stage determination
			// This condition comes from steady state monitoring, not rollout completion
			continue
		}
	}

	// Determine stage based on rollout progress
	if hasRolloutComplete {
		return v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
	} else if hasModelSynced && hasValidated {
		// Both validation and model sync are complete, we're in placement/rollout stage
		return v2pb.DEPLOYMENT_STAGE_PLACEMENT
	} else if hasValidated {
		// Validation complete, now doing model sync
		return v2pb.DEPLOYMENT_STAGE_PLACEMENT
	} else {
		// No clear progress indicators, start from validation
		return v2pb.DEPLOYMENT_STAGE_VALIDATION
	}
}

// GetState returns the current deployment state
func (p *Plugin) GetState(ctx context.Context, observability plugins.ObservabilityContext, deployment *v2pb.Deployment) (v2pb.DeploymentStatus, error) {
	// For OSS, we'll return the current status
	return deployment.Status, nil
}

// HealthCheckGate checks if there are issues with the current model rollout
func (p *Plugin) HealthCheckGate(ctx context.Context, observability plugins.ObservabilityContext, deployment *v2pb.Deployment) (bool, error) {
	// For OSS, we'll do a basic health check
	// Check if the inference server is specified
	if deployment.Spec.GetInferenceServer() == nil {
		return false, nil
	}

	// For now, assume healthy - in a real implementation this would check the inference server status
	return true, nil
}

// PopulateDeploymentLogs populates the deployment logs with error logs
func (p *Plugin) PopulateDeploymentLogs(ctx context.Context, runtimeContext plugins.RequestContext, deployment *v2pb.Deployment) {
	// For OSS, this is a no-op since we don't have log aggregation
	runtimeContext.Logger.Info("PopulateDeploymentLogs called", "deployment", deployment.Name)
}

// PopulateMessage populates the deployment message with error information
func (p *Plugin) PopulateMessage(ctx context.Context, runtimeContext plugins.RequestContext, deployment *v2pb.Deployment) {
	// For OSS, set a basic message
	if deployment.Status.Message == "" {
		deployment.Status.Message = "Deployment processed by OSS plugin"
	}
}

// HandleCleanup handles cleanup when a deployment is being deleted, including ConfigMaps and other resources
func (p *Plugin) HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("OSS Plugin: Starting cleanup for deployment", "deployment", deployment.Name)

	// Use the rollout plugin for ConfigMap cleanup since it has the ConfigMapProvider
	rolloutPlugin, err := p.GetRolloutPlugin(ctx, deployment)
	if err != nil {
		logger.Error(err, "Failed to get rollout plugin for cleanup")
		return err
	}

	// Cast to RolloutPlugin to access HandleCleanup method
	if ossRolloutPlugin, ok := rolloutPlugin.(*RolloutPlugin); ok {
		if err := ossRolloutPlugin.HandleCleanup(ctx, logger, deployment); err != nil {
			logger.Error(err, "Rollout plugin cleanup failed")
			return err
		}
	}

	// Additional cleanup can be done with other plugins if needed
	cleanupPlugin := p.GetCleanupPlugin()
	if ossCleanupPlugin, ok := cleanupPlugin.(*OSSConditionsPlugin); ok {
		// For now, just log that cleanup plugin was called
		// In a real implementation, this would handle additional cleanup
		logger.Info("Cleanup plugin called", "actors", len(ossCleanupPlugin.actors))
	}

	logger.Info("OSS Plugin: Cleanup completed successfully", "deployment", deployment.Name)
	return nil
}

// OSSRolloutConditionsPlugin implements the conditions plugin for OSS rollout using strategies
type OSSRolloutConditionsPlugin struct {
	client     client.Client
	gateway    gateways.Gateway
	blobstore  *blobstore.BlobStore
	logger     logr.Logger
	deployment *v2pb.Deployment
}

// GetActors returns the OSS rollout actors using the strategies pattern
func (p *OSSRolloutConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	params := strategies.Params{
		Client:  p.client,
		Gateway: p.gateway,
		Logger:  p.logger,
	}

	actors := strategies.GetRollingActors(params, p.deployment)

	// Convert to the expected interface
	result := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], len(actors))
	for i, actor := range actors {
		result[i] = &ActorWrapper{actor: actor}
	}

	return result
}

// GetConditions returns the conditions from the deployment status
func (p *OSSRolloutConditionsPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *OSSRolloutConditionsPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existing := range resource.Status.Conditions {
		if existing.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// ActorWrapper wraps the OSS actor to match the expected interface
type ActorWrapper struct {
	actor plugins.ConditionActor
}

// Run wraps the actor run method to match the expected signature
func (w *ActorWrapper) Run(ctx context.Context, resource *v2pb.Deployment, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	// Call the underlying actor's Run method (new signature: returns (*apipb.Condition, error))
	return w.actor.Run(ctx, resource, previousCondition)
}

// GetType returns the actor type
func (w *ActorWrapper) GetType() string {
	return w.actor.GetType()
}

// OSSConditionsPlugin is a simple conditions plugin for OSS actors
type OSSConditionsPlugin struct {
	actors []plugins.ConditionActor
}

// GetActors returns the OSS actors
func (p *OSSConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	// Convert to the expected interface
	result := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], len(p.actors))
	for i, actor := range p.actors {
		result[i] = &ActorWrapper{actor: actor}
	}
	return result
}

// GetConditions returns the conditions from the deployment status
func (p *OSSConditionsPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *OSSConditionsPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existing := range resource.Status.Conditions {
		if existing.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}
