package strategies

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// GetShadowActors returns actors for shadow rollout strategy (canary with analysis)
func GetShadowActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&ShadowDeploymentActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
		&ShadowAnalysisActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
		&ShadowPromotionActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
	}
}

// ShadowDeploymentActor deploys shadow version alongside production
type ShadowDeploymentActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *ShadowDeploymentActor) GetType() string {
	return common.ActorTypeShadowDeployment
}

func (a *ShadowDeploymentActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *ShadowDeploymentActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if shadow deployment is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "ShadowDeploymentCompleted",
			Message: "Shadow deployment completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ShadowDeploymentPending",
		Message: "Shadow deployment has not started yet",
	}, nil
}

func (a *ShadowDeploymentActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running shadow deployment for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting shadow deployment",
			"model", modelName,
			"inference_server", inferenceServerName)

		// Deploy shadow version with traffic splitting
		// In OSS implementation, use Istio VirtualService with weighted routing
		shadowRequest := gateways.ProxyConfigRequest{
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			ModelName:       modelName,
			BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton
			Routes: []gateways.RouteConfig{
				{
					Path:        fmt.Sprintf("/%s-endpoint/%s/production", inferenceServerName, inferenceServerName),
					Destination: fmt.Sprintf("%s-service.%s.svc.cluster.local", inferenceServerName, resource.Namespace),
					Weight:      90, // 90% to production
				},
				{
					Path:        fmt.Sprintf("/%s-endpoint/%s/shadow", inferenceServerName, inferenceServerName),
					Destination: fmt.Sprintf("%s-shadow-service.%s.svc.cluster.local", inferenceServerName, resource.Namespace),
					Weight:      10, // 10% to shadow
				},
			},
		}

		if err := a.gateway.ConfigureProxy(ctx, a.logger, shadowRequest); err != nil {
			a.logger.Error(err, "Failed to configure shadow routing")
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ShadowDeploymentFailed",
				Message: fmt.Sprintf("Failed to configure shadow routing: %v", err),
			}, nil
		}

		// Simulate shadow deployment completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Shadow deployment completed", "model", modelName, "trafficSplit", "10%")
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// ShadowAnalysisActor analyzes shadow deployment results
type ShadowAnalysisActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *ShadowAnalysisActor) GetType() string {
	return common.ActorTypeShadowAnalysis
}

func (a *ShadowAnalysisActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *ShadowAnalysisActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ShadowAnalysisPending",
		Message: "Shadow analysis has not started yet",
	}, nil
}

func (a *ShadowAnalysisActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running shadow analysis for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting shadow analysis",
			"model", modelName,
			"inference_server", inferenceServerName)

		// For OSS, simulate successful analysis
		// In Uber's implementation, this would integrate with DPQS for sophisticated analysis
		a.logger.Info("Shadow analysis completed successfully", "model", modelName)

		// Simulate analysis completion
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// ShadowPromotionActor promotes shadow to production if analysis passes
type ShadowPromotionActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *ShadowPromotionActor) GetType() string {
	return common.ActorTypeShadowPromotion
}

func (a *ShadowPromotionActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *ShadowPromotionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ShadowPromotionPending",
		Message: "Shadow promotion has not started yet",
	}, nil
}

func (a *ShadowPromotionActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running shadow promotion for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting shadow promotion",
			"model", modelName,
			"inference_server", inferenceServerName)

		// Promote shadow to production (100% traffic)
		promotionRequest := gateways.ModelConfigUpdateRequest{
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton
			ModelConfigs: []gateways.ModelConfigEntry{
				{
					Name:   modelName,
					S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
				},
			},
		}

		if err := a.gateway.UpdateModelConfig(ctx, a.logger, promotionRequest); err != nil {
			a.logger.Error(err, "Failed to promote shadow to production")
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ShadowPromotionFailed",
				Message: fmt.Sprintf("Failed to promote shadow to production: %v", err),
			}, nil
		}

		// Simulate promotion completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Shadow promotion completed", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
