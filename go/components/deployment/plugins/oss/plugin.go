package oss

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/cleanup"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollback"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/steadystate"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Subtype the subtype that the plugin represents.
const Subtype = "oss"

var _ plugins.Plugin = &Plugin{}

// Plugin is the OSS plugin implementation
type Plugin struct {
	client                 client.Client
	proxyProvider          proxy.ProxyProvider
	gateway                gateways.Gateway
	blobstore              *blobstore.BlobStore
	logger                 *zap.Logger
	modelConfigMapProvider configmap.ModelConfigMapProvider

	rolloutPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	rollbackPlugin    conditionInterfaces.Plugin[*v2pb.Deployment]
	cleanupPlugin     conditionInterfaces.Plugin[*v2pb.Deployment]
	steadyStatePlugin conditionInterfaces.Plugin[*v2pb.Deployment]
}

// Params contains dependencies for OSS plugin
type Params struct {
	fx.In

	Registrar              pluginmanager.Registrar[plugins.Plugin]
	Client                 client.Client
	Gateway                gateways.Gateway
	ProxyProvider          proxy.ProxyProvider
	BlobStore              *blobstore.BlobStore
	Logger                 *zap.Logger
	ModelConfigMapProvider configmap.ModelConfigMapProvider
}

// NewPlugin creates a new instance of OSS plugin
func NewPlugin(params Params) *Plugin {
	return &Plugin{
		client:                 params.Client,
		gateway:                params.Gateway,
		proxyProvider:          params.ProxyProvider,
		blobstore:              params.BlobStore,
		logger:                 params.Logger,
		modelConfigMapProvider: params.ModelConfigMapProvider,
		rollbackPlugin: rollback.NewRollbackPlugin(rollback.Params{
			Gateway: params.Gateway,
			Logger:  params.Logger,
		}),
		cleanupPlugin: cleanup.NewCleanupPlugin(cleanup.Params{
			ProxyProvider:          params.ProxyProvider,
			Gateway:                params.Gateway,
			Logger:                 params.Logger,
			ModelConfigMapProvider: params.ModelConfigMapProvider,
		}),
		steadyStatePlugin: steadystate.NewSteadyStatePlugin(steadystate.Params{
			Gateway: params.Gateway,
			Logger:  params.Logger,
		}),
	}
}

// GetRolloutPlugin returns the rollout plugin using the OSS rollout conditions plugin
func (p *Plugin) GetRolloutPlugin(ctx context.Context, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	rolloutPlugin, err := rollout.NewRolloutPlugin(ctx, rollout.Params{
		Client:                 p.client,
		ProxyProvider:          p.proxyProvider,
		ModelConfigMapProvider: p.modelConfigMapProvider,
		Gateway:                p.gateway,
		Logger:                 p.logger,
	}, deployment)
	if err != nil {
		return nil, err
	}
	p.rolloutPlugin = rolloutPlugin
	return rolloutPlugin, nil
}

// GetRollbackPlugin returns the rollback plugin
func (p *Plugin) GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.rollbackPlugin
}

// GetCleanupPlugin returns the cleanup plugin
func (p *Plugin) GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.cleanupPlugin
}

// GetSteadyStatePlugin returns the steady state plugin
func (p *Plugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.steadyStatePlugin
}

// ParseStage goes through all the conditions and determines the current deployment stage
func (p *Plugin) ParseStage(deployment *v2pb.Deployment) v2pb.DeploymentStage {
	fmt.Printf("DEBUG: ParseStage CALLED for %s, DesiredRevision=%v, CandidateRevision=%v, conditions=%d\n",
		deployment.Name, deployment.Spec.DesiredRevision, deployment.Status.CandidateRevision, len(deployment.Status.Conditions))

	currentStage := deployment.Status.Stage
	// Check if we need to trigger a new rollout despite having conditions
	// This happens when desired != candidate, which means a new rollout should start
	if deployment.Spec.DesiredRevision != nil && deployment.Status.CandidateRevision != nil {
		if deployment.Spec.DesiredRevision.Name != deployment.Status.CandidateRevision.Name {
			// New rollout needed - start from validation regardless of existing conditions
			return v2pb.DEPLOYMENT_STAGE_VALIDATION
		}
	}

	if len(deployment.Status.Conditions) == 0 {
		return currentStage
	}

	// Check for actual rollout completion conditions (not just steady state)
	hasRolloutComplete := false
	hasValidated := false

	for _, cond := range deployment.Status.Conditions {
		switch cond.Type {
		// case "RolloutComplete":
		case common.ActorTypeRolloutCompletion:
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				hasRolloutComplete = true
			}
		// case "Validated":
		case common.ActorTypeValidation:
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				hasValidated = true
			} else if cond.Status == apipb.CONDITION_STATUS_FALSE {
				return v2pb.DEPLOYMENT_STAGE_VALIDATION
			}
		// case "ModelSynced":
		case common.ActorTypeModelSync:
			if cond.Status == apipb.CONDITION_STATUS_FALSE {
				return v2pb.DEPLOYMENT_STAGE_PLACEMENT
			}
		// case "CleanupComplete":
		case common.ActorTypeCleanup:
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
			} else {
				return v2pb.DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS
			}
		// case "RollbackComplete":
		case common.ActorTypeRollback:
			if cond.Status == apipb.CONDITION_STATUS_TRUE {
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
			} else {
				return v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
			}
		// case "StateSteady":
		case common.ActorTypeSteadyState:
			return currentStage
		}
	}

	// Determine stage based on rollout progress
	if hasRolloutComplete {
		return v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
	} else if hasValidated {
		// Validation has completed, we are in placement stage
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

	// Check if the inference server is healthy
	healthy, err := p.gateway.IsHealthy(ctx, p.logger, gateways.HealthCheckRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check health of inference server: %w", err)
	}
	return healthy, nil
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
