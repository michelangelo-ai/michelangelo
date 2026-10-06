package oss

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/cleanup"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/metricgate"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollback"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/steadystate"
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
	client              client.Client
	apiHandler          goapi.Handler
	httpClient          *http.Client
	dynamicClient       dynamic.Interface
	clientFactory       clientfactory.ClientFactory
	routeManager        routing.Manager
	backendRegistry     *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	blobstore           *blobstore.BlobStore
	logger              *zap.Logger
	config              maconfig.DeploymentConfig
	metricGate          *metricgate.Gate

	rolloutPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	cleanupPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	steadyStatePlugin conditionInterfaces.Plugin[*v2pb.Deployment]
}

// Params contains dependencies injected via Fx for OSS plugin initialization.
type Params struct {
	fx.In

	Registrar           pluginmanager.Registrar[plugins.Plugin]
	Client              client.Client
	APIHandler          goapi.Handler
	HTTPClient          *http.Client
	DynamicClient       dynamic.Interface
	ClientFactory       clientfactory.ClientFactory
	BackendRegistry     *backends.Registry
	RouteManager        routing.Manager
	BlobStore           *blobstore.BlobStore
	Logger              *zap.Logger
	ModelConfigProvider modelconfig.ModelConfigProvider
	Config              maconfig.DeploymentConfig
}

// NewPlugin creates an OSS deployment plugin with rollback, cleanup, and steady state workflows.
func NewPlugin(params Params) *Plugin {
	return &Plugin{
		client:              params.Client,
		apiHandler:          params.APIHandler,
		httpClient:          params.HTTPClient,
		dynamicClient:       params.DynamicClient,
		clientFactory:       params.ClientFactory,
		backendRegistry:     params.BackendRegistry,
		routeManager:        params.RouteManager,
		modelConfigProvider: params.ModelConfigProvider,
		blobstore:           params.BlobStore,
		logger:              params.Logger,
		config:              params.Config,
		metricGate:          metricgate.New(params.Config.MetricGate, params.Logger),
		cleanupPlugin: cleanup.NewCleanupPlugin(cleanup.Params{
			Client:              params.Client,
			DynamicClient:       params.DynamicClient,
			ClientFactory:       params.ClientFactory,
			RouteManager:        params.RouteManager,
			ModelConfigProvider: params.ModelConfigProvider,
			Logger:              params.Logger,
		}),
		steadyStatePlugin: steadystate.NewSteadyStatePlugin(steadystate.Params{
			Logger: params.Logger,
		}),
	}
}

// GetRolloutPlugin creates a deployment-specific rollout plugin with the appropriate strategy.
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	rolloutPlugin, err := rollout.NewRolloutPlugin(ctx, rollout.Params{
		Client:              p.client,
		APIHandler:          p.apiHandler,
		HTTPClient:          p.httpClient,
		DynamicClient:       p.dynamicClient,
		ClientFactory:       p.clientFactory,
		RouteManager:        p.routeManager,
		BackendRegistry:     p.backendRegistry,
		ModelConfigProvider: p.modelConfigProvider,
		Logger:              p.logger,
		Settings:            common.ResolveRolloutSettings(p.config, deployment),
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

// GetRollbackPlugin creates a deployment-specific plugin that reverts every cluster the
// rollout reached to the previous stable revision.
func (p *Plugin) GetRollbackPlugin(ctx context.Context, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	rollbackPlugin, err := rollback.NewRollbackPlugin(ctx, rollback.Params{
		ClientFactory:       p.clientFactory,
		APIHandler:          p.apiHandler,
		BackendRegistry:     p.backendRegistry,
		ModelConfigProvider: p.modelConfigProvider,
		RouteManager:        p.routeManager,
		DynamicClient:       p.dynamicClient,
		Logger:              p.logger,
		Settings:            common.ResolveRolloutSettings(p.config, deployment),
	}, deployment)
	if err != nil {
		p.logger.Error("failed to create rollback plugin",
			zap.Error(err),
			zap.String("operation", "get_rollback_plugin"),
			zap.String("namespace", deployment.Namespace),
			zap.String("deployment", deployment.Name))
		return nil, fmt.Errorf("create rollback plugin for deployment %s/%s: %w",
			deployment.Namespace, deployment.Name, err)
	}
	return rollbackPlugin, nil
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
		switch {
		case cond.Type == common.ActorTypeValidation, cond.Type == common.ActorTypeAssetPreparation:
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		case cond.Type == common.ActorTypeCleanup:
			return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
		case isRollbackCondition(cond.Type):
			return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
		default:
			return v2pb.DEPLOYMENT_STAGE_PLACEMENT
		}
	}
	return stage
}

// isRollbackCondition matches the terminal rollback condition and the per-cluster ones,
// whose types carry a "-<cluster>" suffix.
func isRollbackCondition(conditionType string) bool {
	return conditionType == common.ActorTypeRollback || strings.HasPrefix(conditionType, common.ActorTypeRollback+"-")
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

	// The deployment is healthy when the revision it serves is loaded on every replica in
	// every cluster. That is the current revision, except that during a rollout the clusters
	// that already flipped serve the candidate, and at the very end of a rollout the current
	// revision is unloaded just before the candidate graduates. So either revision being
	// fully loaded counts as healthy.
	models := []string{currentRevision.GetName()}
	if candidate := deployment.Status.GetCandidateRevision().GetName(); candidate != "" && candidate != currentRevision.GetName() {
		models = append(models, candidate)
	}
	healthy := false
	var summaries []string
	for _, model := range models {
		ok, summary, err := common.CheckModelStatusAllClusters(ctx, p.logger, deployment, p.clientFactory, serverBackend, serverName, model)
		if err != nil {
			p.logger.Error("failed to check model status",
				zap.Error(err),
				zap.String("operation", "check_model_status"),
				zap.String("namespace", deployment.Namespace),
				zap.String("deployment", deployment.Name),
				zap.String("model", model))
			return deployment.Status, fmt.Errorf("check model status %s for deployment %s/%s: %w",
				model, deployment.Namespace, deployment.Name, err)
		}
		if ok {
			healthy = true
			break
		}
		summaries = append(summaries, summary)
	}

	if healthy {
		if deployment.Status.GetState() != v2pb.DEPLOYMENT_STATE_HEALTHY {
			p.logger.Info("deployment status changed to healthy",
				zap.String("deployment", deployment.Name),
				zap.String("namespace", deployment.Namespace),
				zap.Strings("models", models),
				zap.String("previous_state", deployment.Status.GetState().String()),
				zap.String("new_state", v2pb.DEPLOYMENT_STATE_HEALTHY.String()))
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}
	} else {
		if deployment.Status.GetState() != v2pb.DEPLOYMENT_STATE_UNHEALTHY {
			p.logger.Info("deployment status changed to unhealthy",
				zap.String("deployment", deployment.Name),
				zap.String("namespace", deployment.Namespace),
				zap.Strings("models", models),
				zap.String("previous_state", deployment.Status.GetState().String()),
				zap.String("new_state", v2pb.DEPLOYMENT_STATE_UNHEALTHY.String()),
				zap.String("summary", strings.Join(summaries, "; ")))
			deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		}
	}
	return deployment.Status, nil
}

// HealthCheckGate decides whether an in-progress rollout may continue. It requires the
// inference server to be healthy in every cluster the rollout placed the deployment in,
// then evaluates the metric gate against the candidate. A false result makes the controller
// roll the candidate back; the reason is recorded on the deployment so operators can see
// which cluster or metric tripped it.
func (p *Plugin) HealthCheckGate(ctx context.Context, observability plugins.ObservabilityContext, deployment *v2pb.Deployment) (bool, error) {
	inferenceServer := deployment.Spec.GetInferenceServer()
	// Check if the inference server is specified
	if inferenceServer == nil {
		return false, nil
	}
	serverBackend, err := p.backendRegistry.GetBackend(v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return false, fmt.Errorf("get backend for inference server %s: %w", inferenceServer.GetName(), err)
	}

	targets, err := common.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return false, fmt.Errorf("read target clusters for deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	for _, target := range targets {
		clusterID := target.GetClusterId()
		kubeClient, err := p.clientFactory.GetClient(ctx, target)
		if err != nil {
			return false, fmt.Errorf("get client for cluster %s: %w", clusterID, err)
		}
		healthy, err := serverBackend.IsHealthy(ctx, p.logger, kubeClient, inferenceServer.GetName(), deployment.Namespace)
		if err != nil {
			p.logger.Error("failed to check health of inference server",
				zap.Error(err),
				zap.String("operation", "health_check_gate"),
				zap.String("namespace", deployment.Namespace),
				zap.String("deployment", deployment.Name),
				zap.String("inference_server", inferenceServer.GetName()),
				zap.String("cluster", clusterID))
			return false, fmt.Errorf("check health of inference server %s for deployment %s/%s in cluster %s: %w",
				inferenceServer.GetName(), deployment.Namespace, deployment.Name, clusterID, err)
		}
		if !healthy {
			p.recordGateReason(deployment, fmt.Sprintf("inference server %s is not healthy in cluster %s", inferenceServer.GetName(), clusterID))
			return false, nil
		}
	}

	healthy, reason, err := p.metricGate.Evaluate(ctx, deployment)
	if err != nil {
		return false, fmt.Errorf("evaluate metric gate for deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	if !healthy {
		p.recordGateReason(deployment, reason)
		return false, nil
	}
	delete(deployment.Annotations, common.HealthGateReasonAnnotation)
	return true, nil
}

// recordGateReason logs a failed gate and stores the reason on the deployment.
func (p *Plugin) recordGateReason(deployment *v2pb.Deployment, reason string) {
	p.logger.Warn("health check gate failed",
		zap.String("namespace", deployment.Namespace),
		zap.String("deployment", deployment.Name),
		zap.String("candidate", deployment.Status.GetCandidateRevision().GetName()),
		zap.String("reason", reason))
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[common.HealthGateReasonAnnotation] = reason
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
