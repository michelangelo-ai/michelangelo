package oss

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/discovery"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/cleanup"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollback"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/steadystate"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	prodEnvironment = "production"
	environmentKey  = "environment"
	Subtype         = "oss"
)

var _ plugins.Plugin = &Plugin{}

// Plugin implements deployment lifecycle management for open-source deployments.
type Plugin struct {
	client                 client.Client
	httpClient             *http.Client
	dynamicClient          dynamic.Interface
	clientFactory          clientfactory.ClientFactory
	routeProvider          route.RouteProvider
	modelDiscoveryProvider discovery.ModelDiscoveryProvider
	backendRegistry        *backends.Registry
	modelConfigProvider    modelconfig.ModelConfigProvider
	blobstore              *blobstore.BlobStore
	logger                 *zap.Logger

	rolloutPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	rollbackPlugin    conditionInterfaces.Plugin[*v2pb.Deployment]
	cleanupPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	steadyStatePlugin conditionInterfaces.Plugin[*v2pb.Deployment]
}

// Params contains dependencies injected via Fx for OSS plugin initialization.
type Params struct {
	fx.In

	Registrar              pluginmanager.Registrar[plugins.Plugin]
	Client                 client.Client
	HTTPClient             *http.Client
	DynamicClient          dynamic.Interface
	ClientFactory          clientfactory.ClientFactory
	BackendRegistry        *backends.Registry
	RouteProvider          route.RouteProvider
	ModelDiscoveryProvider discovery.ModelDiscoveryProvider
	BlobStore              *blobstore.BlobStore
	Logger                 *zap.Logger
	ModelConfigProvider    modelconfig.ModelConfigProvider
}

// NewPlugin creates an OSS deployment plugin with rollback, cleanup, and steady state workflows.
func NewPlugin(params Params) *Plugin {
	return &Plugin{
		client:                 params.Client,
		httpClient:             params.HTTPClient,
		dynamicClient:          params.DynamicClient,
		clientFactory:          params.ClientFactory,
		backendRegistry:        params.BackendRegistry,
		routeProvider:          params.RouteProvider,
		modelDiscoveryProvider: params.ModelDiscoveryProvider,
		modelConfigProvider:    params.ModelConfigProvider,
		blobstore:              params.BlobStore,
		logger:                 params.Logger,
		rollbackPlugin: rollback.NewRollbackPlugin(rollback.Params{
			Client:              params.Client,
			ModelConfigProvider: params.ModelConfigProvider,
			Logger:              params.Logger,
		}),
		cleanupPlugin: cleanup.NewCleanupPlugin(cleanup.Params{
			Client:                 params.Client,
			ClientFactory:          params.ClientFactory,
			RouteProvider:          params.RouteProvider,
			ModelDiscoveryProvider: params.ModelDiscoveryProvider,
			ModelConfigProvider:    params.ModelConfigProvider,
			Logger:                 params.Logger,
		}),
		steadyStatePlugin: steadystate.NewSteadyStatePlugin(steadystate.Params{
			Logger: params.Logger,
		}),
	}
}

// GetRolloutPlugin creates a deployment-specific rollout plugin with the appropriate strategy.
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	rolloutPlugin, err := rollout.NewRolloutPlugin(ctx, rollout.Params{
		Client:                 p.client,
		HTTPClient:             p.httpClient,
		DynamicClient:          p.dynamicClient,
		ClientFactory:          p.clientFactory,
		RouteProvider:          p.routeProvider,
		ModelDiscoveryProvider: p.modelDiscoveryProvider,
		BackendRegistry:        p.backendRegistry,
		ModelConfigProvider:    p.modelConfigProvider,
		Logger:                 p.logger,
	}, deployment)
	if err != nil {
		p.logger.Error("failed to create rollout plugin",
			zap.Error(err),
			zap.String("operation", "get_rollout_plugin"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return nil, fmt.Errorf("create rollout plugin for deployment %s/%s: %w",
			deployment.Namespace, deployment.Name, err)
	}
	p.rolloutPlugin = rolloutPlugin
	return rolloutPlugin, nil
}

// GetRollbackPlugin returns the plugin for reverting to previous stable revision.
func (p *Plugin) GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.rollbackPlugin
}

// GetCleanupPlugin returns the plugin for removing deployment resources.
func (p *Plugin) GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.cleanupPlugin
}

// GetSteadyStatePlugin returns the plugin for monitoring stable deployment operation.
func (p *Plugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.steadyStatePlugin
}

// ParseStage goes through all the conditions and determines the current deployment stage.
func (p *Plugin) ParseStage(deployment *v2pb.Deployment) v2pb.DeploymentStage {
	stage := deployment.Status.Stage

	for _, cond := range deployment.Status.Conditions {
		if p.isFromSteadyState(cond) {
			return stage
		}

		// if a terminal actor has true status, then we return immediately
		if cond.Status == apipb.CONDITION_STATUS_TRUE {
			switch cond.Type {
			case common.ActorTypeRolloutComplete:
				return v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
			case common.ActorTypeCleanup:
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
			case common.ActorTypeRollback:
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
			}
			continue
		}

		// otherwise return the stage based on the first actor with false status
		switch cond.Type {
		case common.ActorTypeValidation:
			fallthrough
		case common.ActorTypeAssetPreparation:
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		case common.ActorTypeCleanup:
			return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		case common.ActorTypeRollback:
			return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
		default:
			return v2pb.DEPLOYMENT_STAGE_PLACEMENT
		}
	}
	return stage
}

// isFromSteadyState checks if the condition comes from a steady state plugin actor
func (p *Plugin) isFromSteadyState(condition *apipb.Condition) bool {
	if p.GetSteadyStatePlugin() == nil {
		return false
	}
	for _, actor := range p.GetSteadyStatePlugin().GetActors() {
		if actor.GetType() == condition.GetType() {
			return true
		}
	}
	return false
}

// GetState computes the current deployment state from the resource status.
func (p *Plugin) GetState(ctx context.Context, observability plugins.ObservabilityContext, deployment *v2pb.Deployment) (v2pb.DeploymentStatus, error) {
	// If currentRevision is nil, this means either:
	//   - The model has never been successfully rolled out, or
	//   - The deployment has reached the DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE stage.
	// If the stage is DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE, set the state to DEPLOYMENT_STATE_EMPTY.
	// Otherwise, set the state to DEPLOYMENT_STATE_INITIALIZING.
	currentRevision := deployment.Status.GetCurrentRevision()
	if currentRevision == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
		if deployment.Status.GetStage() == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE {
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_EMPTY
		}
		return deployment.Status, nil
	}

	inferenceServer := deployment.Spec.GetInferenceServer()
	// Every deployment must have an inference server.
	if inferenceServer == nil || inferenceServer.GetName() == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_INVALID
		return deployment.Status, nil
	}
	serverName := inferenceServer.GetName()
	serverBackend, err := p.backendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return deployment.Status, fmt.Errorf("get backend for inference server %s: %w", serverName, err)
	}
	healthy, summary, err := common.CheckModelStatusAllClusters(ctx, p.logger, deployment, p.clientFactory, serverBackend, serverName, deployment.Spec.DesiredRevision.Name)
	if err != nil {
		p.logger.Error("failed to check model status",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name),
			zap.String("model", deployment.Spec.DesiredRevision.Name))
		return deployment.Status, fmt.Errorf("check model status %s for deployment %s/%s: %w",
			deployment.Spec.DesiredRevision.Name, deployment.Namespace, deployment.Name, err)
	}
	if healthy {
		if deployment.Status.GetState() != v2pb.DEPLOYMENT_STATE_HEALTHY {
			p.logger.Info("deployment status changed to healthy",
				zap.String("deployment", deployment.Name),
				zap.String("namespace", deployment.Namespace),
				zap.String("model", deployment.Spec.DesiredRevision.Name),
				zap.String("previous_state", deployment.Status.GetState().String()),
				zap.String("new_state", v2pb.DEPLOYMENT_STATE_HEALTHY.String()))
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}
	} else {
		if deployment.Status.GetState() != v2pb.DEPLOYMENT_STATE_UNHEALTHY {
			p.logger.Info("deployment status changed to unhealthy",
				zap.String("deployment", deployment.Name),
				zap.String("namespace", deployment.Namespace),
				zap.String("model", deployment.Spec.DesiredRevision.Name),
				zap.String("previous_state", deployment.Status.GetState().String()),
				zap.String("new_state", v2pb.DEPLOYMENT_STATE_UNHEALTHY.String()),
				zap.String("summary", summary))
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		}
	}
	return deployment.Status, nil
}

// HealthCheckGate verifies the inference server is healthy before allowing rollout to proceed.
func (p *Plugin) HealthCheckGate(ctx context.Context, observability plugins.ObservabilityContext, deployment *v2pb.Deployment) (bool, error) {
	// Check if the inference server is specified
	if deployment.Spec.GetInferenceServer() == nil {
		return false, nil
	}
	// Check if the inference server is healthy
	serverBackend, err := p.backendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return false, fmt.Errorf("get backend for inference server %s: %w", deployment.Spec.GetInferenceServer().Name, err)
	}
	healthy, err := serverBackend.IsHealthy(ctx, p.logger, p.client, deployment.Spec.GetInferenceServer().Name, deployment.Namespace)
	if err != nil {
		p.logger.Error("failed to check health of inference server",
			zap.Error(err),
			zap.String("operation", "health_check_gate"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name),
			zap.String("inference_server", deployment.Spec.GetInferenceServer().Name))
		return false, fmt.Errorf("check health of inference server %s for deployment %s/%s: %w",
			deployment.Spec.GetInferenceServer().Name, deployment.Namespace, deployment.Name, err)
	}
	return healthy, nil
}

// PopulateDeploymentLogs adds error logs to deployment status (no-op for OSS).
func (p *Plugin) PopulateDeploymentLogs(ctx context.Context, runtimeContext plugins.RequestContext, deployment *v2pb.Deployment) {
	// For OSS, this is a no-op since we don't have log aggregation
	runtimeContext.Logger.Info("PopulateDeploymentLogs called", "deployment", deployment.Name)
}

// PopulateMessage sets the deployment status message if not already populated.
func (p *Plugin) PopulateMessage(ctx context.Context, runtimeContext plugins.RequestContext, deployment *v2pb.Deployment) {
	// For OSS, set a basic message
	if deployment.Status.Message == "" {
		deployment.Status.Message = "Deployment processed by OSS plugin"
	}
}
