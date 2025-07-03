package oss

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Subtype the subtype that the plugin represents.
const Subtype = "oss"

var _ plugins.Plugin = &Plugin{}

// Plugin is the OSS plugin implementation
type Plugin struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

// NewPlugin creates a new instance of OSS plugin
func NewPlugin(client client.Client, gateway inferenceserver.Gateway, logger logr.Logger) *Plugin {
	return &Plugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

// GetRolloutPlugin returns the rollout plugin
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (plugins.ConditionsPlugin, error) {
	// For OSS, return a simple rollout plugin
	return NewRolloutPlugin(p.client, p.gateway, p.logger), nil
}

// GetRollbackPlugin returns the rollback plugin
func (p *Plugin) GetRollbackPlugin() plugins.ConditionsPlugin {
	return NewRollbackPlugin(p.client, p.gateway, p.logger)
}

// GetCleanupPlugin returns the cleanup plugin
func (p *Plugin) GetCleanupPlugin() plugins.ConditionsPlugin {
	return NewCleanupPlugin(p.client, p.gateway, p.logger)
}

// GetSteadyStatePlugin returns the steady state plugin
func (p *Plugin) GetSteadyStatePlugin() plugins.ConditionsPlugin {
	return NewSteadyStatePlugin(p.client, p.gateway, p.logger)
}

// ParseStage goes through all the conditions and determines the current deployment stage
func (p *Plugin) ParseStage(deployment *v2pb.Deployment) v2pb.DeploymentStage {
	stage := deployment.Status.Stage

	for _, cond := range deployment.Status.Conditions {
		// if the terminal actor has true status, we can return immediately
		if cond.Status == apipb.CONDITION_STATUS_TRUE {
			switch cond.Type {
			case "RolloutComplete":
				return v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
			case "CleanupComplete":
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
			case "RollbackComplete":
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
			}
			continue
		}

		// otherwise return the stage based on the first actor with false status
		switch cond.Type {
		case "Validated":
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		case "CleanupComplete":
			return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		case "RollbackComplete":
			return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
		default:
			return v2pb.DEPLOYMENT_STAGE_PLACEMENT
		}
	}
	return stage
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