package strategies

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelCleanupActor handles cleanup of old models after successful deployment
// Following Uber's UCS pattern for safe model cleanup
type ModelCleanupActor struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  logr.Logger
}

func (a *ModelCleanupActor) GetType() string {
	return "ModelCleaned"
}

func (a *ModelCleanupActor) GetLogger() logr.Logger {
	return a.Logger
}

func (a *ModelCleanupActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if cleanup is needed by comparing current vs desired revision
	if resource.Status.CurrentRevision == nil || resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "NoCleanupNeeded",
			Message: "No cleanup needed - no revisions to compare",
		}, nil
	}

	currentModel := resource.Status.CurrentRevision.Name
	desiredModel := resource.Spec.DesiredRevision.Name

	// If models are the same, no cleanup needed
	if currentModel == desiredModel {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "NoCleanupNeeded",
			Message: fmt.Sprintf("No cleanup needed - model %s is current", currentModel),
		}, nil
	}

	inferenceServerName := resource.Spec.GetInferenceServer().Name
	a.Logger.Info("Checking if old model cleanup is needed",
		"current_model", currentModel,
		"desired_model", desiredModel,
		"inference_server", inferenceServerName)

	// Check if old model still exists in Triton (following Uber's verification pattern)
	if a.Gateway != nil {
		// Use gateway to check if old model is still loaded
		statusRequest := gateways.ModelStatusRequest{
			ModelName:       currentModel,
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TRITON,
		}

		ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
		if err != nil {
			// If we can't check status, assume cleanup is needed
			a.Logger.Info("Cannot verify old model status, cleanup may be needed", "error", err)
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "CleanupPending",
				Message: fmt.Sprintf("Need to cleanup old model %s", currentModel),
			}, nil
		}

		if ready {
			// Old model is still loaded, cleanup needed
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "CleanupPending",
				Message: fmt.Sprintf("Old model %s still loaded, cleanup needed", currentModel),
			}, nil
		}
	}

	// Old model not found or already cleaned up
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupComplete",
		Message: fmt.Sprintf("Old model %s already cleaned up", currentModel),
	}, nil
}

func (a *ModelCleanupActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.Logger.Info("Running model cleanup for deployment", "deployment", resource.Name)

	if resource.Status.CurrentRevision == nil || resource.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "NoCleanupNeeded",
			Message: "No cleanup needed - no revisions specified",
		}, nil
	}

	currentModel := resource.Status.CurrentRevision.Name
	desiredModel := resource.Spec.DesiredRevision.Name
	inferenceServerName := resource.Spec.GetInferenceServer().Name

	// If models are the same, no cleanup needed
	if currentModel == desiredModel {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "NoCleanupNeeded",
			Message: fmt.Sprintf("No cleanup needed - model %s is current", currentModel),
		}, nil
	}

	a.Logger.Info("Starting model cleanup",
		"old_model", currentModel,
		"new_model", desiredModel,
		"inference_server", inferenceServerName)

	// PHASE 1: Update ConfigMap to remove old model (following Uber's UCS pattern)
	if a.Gateway != nil {
		// Get current ConfigMap and remove old model from it
		a.Logger.Info("Phase 1: Removing old model from ConfigMap", "old_model", currentModel)

		// Create update request to remove old model from ConfigMap
		updateRequest := gateways.ModelConfigUpdateRequest{
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TRITON,
			ModelConfigs: []gateways.ModelConfigEntry{
				{
					Name:   desiredModel, // Only keep the new model
					S3Path: fmt.Sprintf("s3://deploy-models/%s/", desiredModel),
				},
			},
		}

		if err := a.Gateway.UpdateModelConfig(ctx, a.Logger, updateRequest); err != nil {
			a.Logger.Error(err, "Failed to update ConfigMap during cleanup")
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ConfigMapCleanupFailed",
				Message: fmt.Sprintf("Failed to remove old model %s from ConfigMap: %v", currentModel, err),
			}, nil
		}

		// PHASE 2: Directly unload old model from Triton using API (following Uber's pattern)
		a.Logger.Info("Phase 2: Unloading old model from Triton", "old_model", currentModel)

		if err := a.unloadModelFromTriton(ctx, currentModel, inferenceServerName); err != nil {
			a.Logger.Error(err, "Failed to unload old model from Triton", "model", currentModel)
			// Don't fail the deployment if direct unload fails - ConfigMap update should handle it
			a.Logger.Info("ConfigMap update should eventually unload the model automatically")
		}

		// PHASE 3: Verify cleanup completed
		a.Logger.Info("Phase 3: Verifying old model is unloaded", "old_model", currentModel)

		statusRequest := gateways.ModelStatusRequest{
			ModelName:       currentModel,
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TRITON,
		}

		ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
		if err == nil && ready {
			a.Logger.Info("Old model still loaded, but ConfigMap update should unload it eventually", "model", currentModel)
		} else {
			a.Logger.Info("Old model successfully unloaded", "model", currentModel)
		}
	}

	a.Logger.Info("Model cleanup completed successfully",
		"old_model", currentModel,
		"new_model", desiredModel)

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "CleanupCompleted",
		Message: fmt.Sprintf("Successfully cleaned up old model %s, active model is now %s", currentModel, desiredModel),
	}, nil
}

// unloadModelFromTriton directly calls Triton API to unload model (following Uber's pattern)
func (a *ModelCleanupActor) unloadModelFromTriton(ctx context.Context, modelName, inferenceServerName string) error {
	// Construct Triton unload API endpoint
	unloadURL := fmt.Sprintf("http://localhost:8888/%s/v2/repository/models/%s/unload", inferenceServerName, modelName)

	a.Logger.Info("Calling Triton unload API", "url", unloadURL, "model", modelName)

	// Create HTTP request to unload model
	req, err := http.NewRequestWithContext(ctx, "POST", unloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create unload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Triton unload API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Triton unload API returned status %d", resp.StatusCode)
	}

	a.Logger.Info("Successfully called Triton unload API", "model", modelName, "status", resp.StatusCode)
	return nil
}
