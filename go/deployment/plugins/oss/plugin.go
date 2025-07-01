package oss

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/cleanup"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/rollback"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/rollout"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/steadystate"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Subtype defines the plugin subtype
const Subtype = "oss"

// ObservabilityContext provides observability tools for plugins
type ObservabilityContext struct {
	Logger logr.Logger
}

// Plugin represents the OSS deployment plugin
type Plugin struct {
	gateway inferenceserver.Gateway
}

// NewPlugin creates a new OSS deployment plugin
func NewPlugin(gateway inferenceserver.Gateway) *Plugin {
	return &Plugin{
		gateway: gateway,
	}
}

// GetRolloutPlugin returns the rollout plugin for OSS deployments
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (RolloutPlugin, error) {
	return rollout.NewPlugin(p.gateway), nil
}

// GetRollbackPlugin returns the rollback plugin for OSS deployments
func (p *Plugin) GetRollbackPlugin() RollbackPlugin {
	return rollback.NewPlugin(p.gateway)
}

// GetCleanupPlugin returns the cleanup plugin for OSS deployments
func (p *Plugin) GetCleanupPlugin() CleanupPlugin {
	return cleanup.NewPlugin(p.gateway)
}

// GetSteadyStatePlugin returns the steady state plugin for OSS deployments
func (p *Plugin) GetSteadyStatePlugin() SteadyStatePlugin {
	return steadystate.NewPlugin(p.gateway)
}

// ParseStage determines the current deployment stage based on conditions
func (p *Plugin) ParseStage(deployment *v2pb.Deployment) v2pb.DeploymentStage {
	stage := deployment.Status.Stage
	
	// If no conditions are set, return current stage
	if len(deployment.Status.Conditions) == 0 {
		return stage
	}
	
	// Check conditions to determine stage
	for _, condition := range deployment.Status.Conditions {
		// If condition is true, check for terminal states
		if condition.Status == apipb.CONDITION_STATUS_TRUE {
			switch condition.Type {
			case common.ActorTypeHealthCheck:
				if stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {
					return v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
				}
			case common.ActorTypeCleanup:
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
			case common.ActorTypeRollback:
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
			}
			continue
		}
		
		// If condition is false, determine stage based on actor type
		switch condition.Type {
		case common.ActorTypeValidation:
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		case common.ActorTypeResourcePrep:
			return v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION
		case common.ActorTypeModelLoad,
			 common.ActorTypeHealthCheck:
			return v2pb.DEPLOYMENT_STAGE_PLACEMENT
		case common.ActorTypeCleanup:
			return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		case common.ActorTypeRollback:
			return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
		}
	}
	
	return stage
}

// HealthCheckGate performs health checks for the deployment
func (p *Plugin) HealthCheckGate(ctx context.Context, observability ObservabilityContext, deployment *v2pb.Deployment) (bool, error) {
	if deployment.Status.CurrentRevision == nil {
		return true, nil // No current revision to check
	}
	
	inferenceServerName := common.GetInferenceServerName(*deployment)
	if inferenceServerName == "" {
		return false, fmt.Errorf("no inference server specified")
	}
	
	// Check if inference server is healthy
	isHealthy, err := p.gateway.IsHealthy(ctx, observability.Logger, inferenceServerName, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		observability.Logger.Error(err, "Failed to check inference server health")
		return false, err
	}
	
	return isHealthy, nil
}

// GetState retrieves the current state of the deployment
func (p *Plugin) GetState(ctx context.Context, observability ObservabilityContext, deployment *v2pb.Deployment) (v2pb.DeploymentStatus, error) {
	status := deployment.Status
	
	// Update state based on current conditions
	if len(status.Conditions) > 0 {
		// Check if all conditions are true (successful deployment)
		allTrue := true
		for _, condition := range status.Conditions {
			if condition.Status != apipb.CONDITION_STATUS_TRUE {
				allTrue = false
				break
			}
		}
		
		if allTrue && status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
			status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		} else if status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED ||
				  status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED ||
				  status.Stage == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED {
			status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		} else {
			status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
		}
	}
	
	return status, nil
}

// PopulateDeploymentLogs populates deployment logs (no-op for OSS)
func (p *Plugin) PopulateDeploymentLogs(ctx context.Context, runtimeContext interface{}, deployment *v2pb.Deployment) {
	// OSS implementation doesn't require log population
}

// PopulateMessage populates deployment message (no-op for OSS)
func (p *Plugin) PopulateMessage(ctx context.Context, runtimeContext interface{}, deployment *v2pb.Deployment) {
	// OSS implementation uses default messages
}

// Plugin interfaces
type RolloutPlugin interface {
	Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
	GetActors() []common.Actor
}

type RollbackPlugin interface {
	Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
	GetActors() []common.Actor
}

type CleanupPlugin interface {
	Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
	GetActors() []common.Actor
}

type SteadyStatePlugin interface {
	Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
	GetActors() []common.Actor
}