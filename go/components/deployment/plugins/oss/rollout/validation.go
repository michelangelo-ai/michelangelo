package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ValidationActor validates deployment configuration
type ValidationActor struct {
	client client.Client
	logger *zap.Logger
}

func (a *ValidationActor) GetType() string {
	return common.ActorTypeValidation
}

func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Validate deployment configuration
	if resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for deployment",
		}, nil
	}

	if resource.Spec.GetInferenceServer() == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoInferenceServer",
			Message: "No inference server specified for deployment",
		}, nil
	}

	modelName := resource.Spec.DesiredRevision.Name
	if modelName == "" {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidModelName",
			Message: "Model name cannot be empty",
		}, nil
	}

	// For OSS, validate model exists in available models
	if !common.IsModelAvailable(modelName) {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelNotFound",
			Message: fmt.Sprintf("Model %s not found in storage. Available models: %s", modelName, common.GetAvailableModels()),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: fmt.Sprintf("Deployment validation completed successfully for model %s", modelName),
	}, nil
}

func (a *ValidationActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running validation for deployment", zap.String("deployment", deployment.Name))

	// Update deployment status to show validation is in progress
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_VALIDATION

	// Perform comprehensive validation
	if deployment.Spec.DesiredRevision == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: No desired revision specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoDesiredRevision", Message: "Validation failed: No desired revision specified"}, nil
	}

	if deployment.Spec.GetInferenceServer() == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: No inference server specified"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "NoInferenceServer", Message: "Validation failed: No inference server specified"}, nil
	}

	// Additional OSS-specific validations
	if deployment.Spec.DesiredRevision.Name == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: Desired revision name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyRevisionName", Message: "Validation failed: Desired revision name is empty"}, nil
	}

	if deployment.Spec.GetInferenceServer().Name == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "Validation failed: Inference server name is empty"
		return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "EmptyInferenceServerName", Message: "Validation failed: Inference server name is empty"}, nil
	}

	// If all validations pass
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING
	deployment.Status.Message = "Validation completed successfully"
	a.logger.Info("Validation completed successfully", zap.String("deployment", deployment.Name))

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
