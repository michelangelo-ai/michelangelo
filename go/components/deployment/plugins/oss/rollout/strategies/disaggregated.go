package strategies

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// getBackendTypeFromDeployment determines the backend type from the deployment's inference server
// In a real implementation, this would query the inference server to get its backend type
func getBackendTypeFromDeployment(deployment *v2pb.Deployment) v2pb.BackendType {
	// For now, default to Triton. In a production system, this would:
	// 1. Query the inference server CRD to get its backend type
	// 2. Use the backend type from the deployment spec if available
	// 3. Have proper error handling for unknown backend types
	return v2pb.BACKEND_TYPE_TRITON
}

// DisaggregatedStep represents a step in disaggregated deployment
type DisaggregatedStep struct {
	Name        string
	Environment string
	Strategy    string
	Percentage  int32
	SoakTime    time.Duration
}

// GetDisaggregatedActors returns actors for disaggregated rollout strategy (multi-step deployment)
func GetDisaggregatedActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&DisaggregatedRolloutActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
	}
}

// DisaggregatedRolloutActor implements multi-step, environment-specific deployment
type DisaggregatedRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *DisaggregatedRolloutActor) GetType() string {
	return common.ActorTypeDisaggregatedRollout
}

func (a *DisaggregatedRolloutActor) GetLogger() *zap.Logger {
	return a.logger
}

func (a *DisaggregatedRolloutActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if disaggregated rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "DisaggregatedRolloutCompleted",
			Message: "Disaggregated rollout completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "DisaggregatedRolloutPending",
		Message: "Disaggregated rollout has not started yet",
	}, nil
}

func (a *DisaggregatedRolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running disaggregated rollout for deployment", zap.String("deployment", resource.Name))

	// Update deployment to placement stage
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting disaggregated rollout",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		// Define deployment steps - in real implementation, this would come from deployment spec
		steps := a.getDeploymentSteps(resource)

		for i, step := range steps {
			a.logger.Info("Executing deployment step", zap.Int("step", i+1), zap.String("name", step.Name), zap.String("environment", step.Environment))

			if err := a.executeStep(ctx, resource, step); err != nil {
				a.logger.Error("Failed deployment step", zap.String("step", step.Name), zap.Error(err))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "DisaggregatedRolloutFailed",
					Message: fmt.Sprintf("Failed at step %s: %v", step.Name, err),
				}, nil
			}

			// Wait for soak time between steps (simplified for OSS)
			if step.SoakTime > 0 && i < len(steps)-1 {
				a.logger.Info("Soaking between steps", zap.Duration("duration", step.SoakTime), zap.String("step", step.Name))
				// In real implementation, this would be handled by the controller's reconcile loop
				// For OSS, we skip the soak time to simplify
			}
		}

		// Simulate disaggregated rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Disaggregated rollout completed successfully", zap.String("model", modelName), zap.Int("totalSteps", len(steps)))
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

func (a *DisaggregatedRolloutActor) getDeploymentSteps(deployment *v2pb.Deployment) []DisaggregatedStep {
	// In OSS implementation, define standard disaggregated steps
	// In Uber's implementation, this would come from deployment configuration
	return []DisaggregatedStep{
		{
			Name:        "canary-deployment",
			Environment: "staging",
			Strategy:    "zonal",
			Percentage:  10,
			SoakTime:    2 * time.Minute,
		},
		{
			Name:        "partial-rollout",
			Environment: "production",
			Strategy:    "rolling",
			Percentage:  50,
			SoakTime:    5 * time.Minute,
		},
		{
			Name:        "full-rollout",
			Environment: "production",
			Strategy:    "blast",
			Percentage:  100,
			SoakTime:    0,
		},
	}
}

func (a *DisaggregatedRolloutActor) executeStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep) error {
	a.logger.Info("Executing deployment step", zap.String("step", step.Name), zap.String("strategy", step.Strategy), zap.Int32("percentage", step.Percentage))

	modelName := deployment.Spec.DesiredRevision.Name
	if modelName == "" {
		modelName = deployment.Name
	}

	// Validate model before deployment
	if err := a.validateModel(ctx, deployment, modelName); err != nil {
		return fmt.Errorf("model validation failed for step %s: %w", step.Name, err)
	}

	// Execute step based on strategy
	switch step.Strategy {
	case "zonal":
		return a.executeZonalStep(ctx, deployment, step, modelName)
	case "rolling":
		return a.executeRollingStep(ctx, deployment, step, modelName)
	case "blast":
		return a.executeBlastStep(ctx, deployment, step, modelName)
	default:
		return fmt.Errorf("unknown strategy %s for step %s", step.Strategy, step.Name)
	}
}

func (a *DisaggregatedRolloutActor) validateModel(ctx context.Context, deployment *v2pb.Deployment, modelName string) error {
	// Model load validation
	loadRequest := gateways.ModelLoadRequest{
		ModelName:       modelName,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		PackagePath:     fmt.Sprintf("s3://deploy-models/%s/", modelName),
	}

	if err := a.gateway.LoadModel(ctx, a.logger, loadRequest); err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}

	// Model prediction validation
	statusRequest := gateways.ModelStatusRequest{
		ModelName:       modelName,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
	}

	ready, err := a.gateway.CheckModelStatus(ctx, a.logger, statusRequest)
	if err != nil {
		return fmt.Errorf("failed to check model status: %w", err)
	}

	if !ready {
		return fmt.Errorf("model %s is not ready for predictions", modelName)
	}

	a.logger.Info("Model validation completed", zap.String("model", modelName))
	return nil
}

func (a *DisaggregatedRolloutActor) executeZonalStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep, modelName string) error {
	a.logger.Info("Executing zonal step", zap.String("step", step.Name), zap.Int32("percentage", step.Percentage))

	// Get subset of zones based on percentage
	zones, err := a.getTargetZones(ctx, deployment)
	if err != nil {
		return err
	}

	targetZones := a.selectZonesByPercentage(zones, step.Percentage)

	for _, zone := range targetZones {
		if err := a.deployToZone(ctx, deployment, zone, modelName); err != nil {
			return fmt.Errorf("failed to deploy to zone %s: %w", zone, err)
		}
	}

	return nil
}

func (a *DisaggregatedRolloutActor) executeRollingStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep, modelName string) error {
	a.logger.Info("Executing rolling step", zap.String("step", step.Name), zap.Int32("percentage", step.Percentage))

	// Update model config with percentage-based rollout
	updateRequest := gateways.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []gateways.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	return a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest)
}

func (a *DisaggregatedRolloutActor) executeBlastStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep, modelName string) error {
	a.logger.Info("Executing blast step", zap.String("step", step.Name))

	// Full deployment to all remaining infrastructure
	updateRequest := gateways.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []gateways.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	return a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest)
}

func (a *DisaggregatedRolloutActor) selectZonesByPercentage(zones []string, percentage int32) []string {
	if percentage >= 100 {
		return zones
	}

	targetCount := int(float64(len(zones)) * float64(percentage) / 100.0)
	if targetCount < 1 {
		targetCount = 1
	}

	if targetCount > len(zones) {
		targetCount = len(zones)
	}

	return zones[:targetCount]
}

// Helper methods reused from zonal.go
func (a *DisaggregatedRolloutActor) getTargetZones(ctx context.Context, deployment *v2pb.Deployment) ([]string, error) {
	// Implementation same as ZonalRolloutActor.getTargetZones
	// Would be extracted to common utility in real implementation
	return []string{"zone-1", "zone-2", "zone-3"}, nil
}

func (a *DisaggregatedRolloutActor) deployToZone(ctx context.Context, deployment *v2pb.Deployment, zone string, modelName string) error {
	// Implementation same as ZonalRolloutActor.deployToZone
	// Would be extracted to common utility in real implementation
	updateRequest := gateways.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []gateways.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	return a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest)
}
