package strategies

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		&ModelSyncActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
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
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func (a *DisaggregatedRolloutActor) GetType() string {
	return common.ActorTypeDisaggregatedRollout
}

func (a *DisaggregatedRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, existingCondition *apipb.Condition) (*apipb.Condition, error) {
	condition := &apipb.Condition{
		Type:   "DisaggregatedRollout",
		Status: apipb.CONDITION_STATUS_FALSE,
		Reason: "DisaggregatedRolloutInProgress",
	}

	if existingCondition != nil {
		condition = existingCondition
	}

	a.logger.Info("Retrieved disaggregated rollout condition", "status", condition.Status, "reason", condition.Reason)
	return condition, nil
}

func (a *DisaggregatedRolloutActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *apipb.Condition) error {
	a.logger.Info("Starting disaggregated rollout", "deployment", deployment.Name)

	// Define deployment steps - in real implementation, this would come from deployment spec
	steps := a.getDeploymentSteps(deployment)

	for i, step := range steps {
		a.logger.Info("Executing deployment step", "step", i+1, "name", step.Name, "environment", step.Environment)

		if err := a.executeStep(ctx, deployment, step); err != nil {
			condition.Status = apipb.CONDITION_STATUS_FALSE
			condition.Reason = "DisaggregatedRolloutFailed"
			condition.Message = fmt.Sprintf("Failed at step %s: %v", step.Name, err)
			return err
		}

		// Wait for soak time between steps
		if step.SoakTime > 0 && i < len(steps)-1 {
			a.logger.Info("Soaking between steps", "duration", step.SoakTime, "step", step.Name)
			time.Sleep(step.SoakTime)
		}
	}

	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "DisaggregatedRolloutCompleted"
	condition.Message = fmt.Sprintf("Completed all %d deployment steps successfully", len(steps))

	a.logger.Info("Disaggregated rollout completed", "totalSteps", len(steps))
	return nil
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
	a.logger.Info("Executing deployment step", "step", step.Name, "strategy", step.Strategy, "percentage", step.Percentage)

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
	loadRequest := inferenceserver.ModelLoadRequest{
		ModelName:       modelName,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		BackendType:     getBackendTypeFromDeployment(deployment),
		PackagePath:     fmt.Sprintf("s3://deploy-models/%s/", modelName),
	}

	if err := a.gateway.LoadModel(ctx, a.logger, loadRequest); err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}

	// Model prediction validation
	statusRequest := inferenceserver.ModelStatusRequest{
		ModelName:       modelName,
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		BackendType:     getBackendTypeFromDeployment(deployment),
	}

	ready, err := a.gateway.CheckModelStatus(ctx, a.logger, statusRequest)
	if err != nil {
		return fmt.Errorf("failed to check model status: %w", err)
	}

	if !ready {
		return fmt.Errorf("model %s is not ready for predictions", modelName)
	}

	a.logger.Info("Model validation completed", "model", modelName)
	return nil
}

func (a *DisaggregatedRolloutActor) executeZonalStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep, modelName string) error {
	a.logger.Info("Executing zonal step", "step", step.Name, "percentage", step.Percentage)

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
	a.logger.Info("Executing rolling step", "step", step.Name, "percentage", step.Percentage)

	// Update model config with percentage-based rollout
	updateRequest := inferenceserver.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []inferenceserver.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	return a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest)
}

func (a *DisaggregatedRolloutActor) executeBlastStep(ctx context.Context, deployment *v2pb.Deployment, step DisaggregatedStep, modelName string) error {
	a.logger.Info("Executing blast step", "step", step.Name)

	// Full deployment to all remaining infrastructure
	updateRequest := inferenceserver.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []inferenceserver.ModelConfigEntry{
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
	updateRequest := inferenceserver.ModelConfigUpdateRequest{
		InferenceServer: deployment.Spec.GetInferenceServer().Name,
		Namespace:       deployment.Namespace,
		BackendType:     getBackendTypeFromDeployment(deployment),
		ModelConfigs: []inferenceserver.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	return a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest)
}