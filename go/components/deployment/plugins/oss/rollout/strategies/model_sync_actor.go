package strategies

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelSyncActor handles model synchronization to inference servers
type ModelSyncActor struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  logr.Logger
}

func (a *ModelSyncActor) GetType() string {
	return "ModelSynced"
}

func (a *ModelSyncActor) GetLogger() logr.Logger {
	return a.Logger
}

func (a *ModelSyncActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if the desired model is actually loaded and ready in Triton
	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.Logger.Info("Checking if model is loaded in Triton", "model", modelName, "inference_server", inferenceServerName)

		// Check if model is ready in Triton using the gateway health check
		if a.Gateway != nil {
			statusRequest := gateways.ModelStatusRequest{
				ModelName:       modelName,
				InferenceServer: inferenceServerName,
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
			if err != nil {
				a.Logger.Error(err, "Failed to check model status in Triton", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelStatusCheckFailed",
					Message: fmt.Sprintf("Failed to check model %s status: %v", modelName, err),
				}, nil
			}

			if ready {
				a.Logger.Info("Model is ready in Triton", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_TRUE,
					Reason:  "ModelReady",
					Message: fmt.Sprintf("Model %s is loaded and ready in Triton", modelName),
				}, nil
			}

			a.Logger.Info("Model is not ready in Triton", "model", modelName)
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ModelNotReady",
				Message: fmt.Sprintf("Model %s is not ready in Triton", modelName),
			}, nil
		}

		// For now, assume model sync is needed if gateway is not available
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "ModelSyncPending",
			Message: fmt.Sprintf("Model %s sync is pending", modelName),
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "NoModelSpecified",
		Message: "No model specified for sync",
	}, nil
}

func (a *ModelSyncActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running model sync for deployment", "deployment", resource.Name)

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.Logger.Info("Syncing model to inference server",
			"model", modelName,
			"inference_server", inferenceServerName)

		// Update the ConfigMap with the new model
		if a.Gateway != nil {
			updateRequest := gateways.ModelConfigUpdateRequest{
				InferenceServer: inferenceServerName,
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
				ModelConfigs: []gateways.ModelConfigEntry{
					{
						Name:   modelName,
						S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
					},
				},
			}

			if err := a.Gateway.UpdateModelConfig(ctx, a.Logger, updateRequest); err != nil {
				a.Logger.Error(err, "Failed to update model config via gateway")
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelConfigUpdateFailed",
					Message: fmt.Sprintf("Failed to update model config: %v", err),
				}, nil
			}

			a.Logger.Info("Model configuration updated successfully", "model", modelName)

			// Verify model is loaded and ready in Triton after config update
			statusRequest := gateways.ModelStatusRequest{
				ModelName:       modelName,
				InferenceServer: inferenceServerName,
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
			if err != nil {
				a.Logger.Error(err, "Failed to verify model status after config update", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelVerificationFailed",
					Message: fmt.Sprintf("Failed to verify model %s after config update: %v", modelName, err),
				}, nil
			}

			if !ready {
				a.Logger.Info("Model is not ready after config update, will retry", "model", modelName)
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelNotReadyAfterUpdate",
					Message: fmt.Sprintf("Model %s is not ready after config update", modelName),
				}, nil
			}

			a.Logger.Info("Model verified as ready after config update", "model", modelName)
		}

		// Update candidate revision to track progress
		resource.Status.CandidateRevision = resource.Spec.DesiredRevision
		a.Logger.Info("Model sync completed successfully")
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "Success",
		Message: "Operation completed successfully",
	}, nil
}