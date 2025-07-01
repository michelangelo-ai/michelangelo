package inferenceserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
)

// LLMD-specific implementations

func (g *gateway) loadLLMDModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading LLMD model", "model", request.ModelName, "server", request.InferenceServer)

	// LLMD doesn't support dynamic model loading like Triton
	// Instead, we need to update the LLMD deployment/configuration
	// and trigger a restart to load the new model

	// For now, this is a placeholder that would:
	// 1. Update LLMD deployment with new model configuration
	// 2. Trigger rolling restart of LLMD pods
	// 3. Wait for pods to come back up with new model

	logger.Info("LLMD model loading initiated (placeholder)", 
		"model", request.ModelName, 
		"packagePath", request.PackagePath)

	// TODO: Implement actual LLMD deployment update logic
	// This would involve:
	// - Updating LLMD Kubernetes deployment spec with new model path
	// - Triggering rolling update of LLMD pods
	// - The new pods will load the model on startup

	return nil
}

func (g *gateway) checkLLMDModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking LLMD model status", "model", request.ModelName, "server", request.InferenceServer)

	// Check if LLMD is serving the expected model
	// This would typically involve calling LLMD's health/status endpoint

	// Build LLMD health URL
	url := fmt.Sprintf("http://%s-endpoint/health", request.InferenceServer)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create LLMD status request: %w", err)
	}

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		logger.Info("Failed to reach LLMD endpoint", "url", url, "error", err)
		return false, nil // Model not ready yet
	}
	defer resp.Body.Close()

	// For LLMD, if the health endpoint returns 200, the model is considered ready
	ready := resp.StatusCode == 200
	logger.Info("LLMD model status checked", "model", request.ModelName, "ready", ready)

	return ready, nil
}

func (g *gateway) getLLMDModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting LLMD model detailed status", "model", request.ModelName, "server", request.InferenceServer)

	// Check LLMD health endpoint for detailed status
	url := fmt.Sprintf("http://%s-endpoint/health", request.InferenceServer)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLMD status request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return &ModelStatus{
			State:   "UNKNOWN",
			Message: fmt.Sprintf("Failed to reach LLMD: %v", err),
			Ready:   false,
		}, nil
	}
	defer resp.Body.Close()

	status := &ModelStatus{}

	switch resp.StatusCode {
	case 200:
		status.State = "LOADED"
		status.Message = "LLMD model loaded and serving"
		status.Ready = true
	case 503:
		status.State = "LOADING"
		status.Message = "LLMD service unavailable, possibly loading model"
		status.Ready = false
	default:
		status.State = "FAILED"
		status.Message = fmt.Sprintf("LLMD returned status: %d", resp.StatusCode)
		status.Ready = false
	}

	logger.Info("LLMD model status retrieved", "model", request.ModelName, "state", status.State)
	return status, nil
}

func (g *gateway) isLLMDHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking LLMD server health", "server", serverName)

	// Build LLMD health URL
	url := fmt.Sprintf("http://%s-endpoint/health", serverName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create LLMD health request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		logger.Info("LLMD health check failed", "server", serverName, "error", err)
		return false, nil
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode == 200
	logger.Info("LLMD health check completed", "server", serverName, "healthy", healthy)

	return healthy, nil
}