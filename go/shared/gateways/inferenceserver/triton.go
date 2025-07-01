package inferenceserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
)

// Triton-specific implementations

func (g *gateway) loadTritonModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading Triton model", "model", request.ModelName, "server", request.InferenceServer)

	// Build Triton model load URL
	url := fmt.Sprintf("http://%s-endpoint/v2/repository/models/%s/load",
		request.InferenceServer, request.ModelName)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create load request: %w", err)
	}

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to load Triton model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Triton model load failed with status: %d", resp.StatusCode)
	}

	logger.Info("Triton model load initiated successfully", "model", request.ModelName)
	return nil
}

func (g *gateway) checkTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking Triton model status", "model", request.ModelName, "server", request.InferenceServer)

	// Build Triton model ready URL
	url := fmt.Sprintf("http://%s-endpoint/v2/models/%s/ready",
		request.InferenceServer, request.ModelName)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create status request: %w", err)
	}

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		logger.Info("Failed to reach Triton endpoint", "url", url, "error", err)
		return false, nil // Model not ready yet, don't return error
	}
	defer resp.Body.Close()

	// Triton returns 200 if model is ready, 400 if not ready, 404 if not found
	ready := resp.StatusCode == http.StatusOK
	logger.Info("Triton model status checked", "model", request.ModelName, "ready", ready)

	return ready, nil
}

func (g *gateway) getTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting Triton model detailed status", "model", request.ModelName, "server", request.InferenceServer)

	// Build Triton model metadata URL
	url := fmt.Sprintf("http://%s-endpoint/v2/models/%s",
		request.InferenceServer, request.ModelName)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return &ModelStatus{
			State:   "UNKNOWN",
			Message: fmt.Sprintf("Failed to reach Triton: %v", err),
			Ready:   false,
		}, nil
	}
	defer resp.Body.Close()

	status := &ModelStatus{}

	switch resp.StatusCode {
	case http.StatusOK:
		// Parse model metadata to get detailed status
		var modelMetadata map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&modelMetadata); err != nil {
			status.State = "LOADED"
			status.Message = "Model loaded but metadata parsing failed"
			status.Ready = true
		} else {
			status.State = "LOADED"
			status.Message = "Model loaded successfully"
			status.Ready = true
		}
	case http.StatusNotFound:
		status.State = "NOT_FOUND"
		status.Message = "Model not found on Triton server"
		status.Ready = false
	case http.StatusBadRequest:
		status.State = "FAILED"
		status.Message = "Model failed to load or is in error state"
		status.Ready = false
	default:
		status.State = "UNKNOWN"
		status.Message = fmt.Sprintf("Unexpected status code: %d", resp.StatusCode)
		status.Ready = false
	}

	logger.Info("Triton model status retrieved", "model", request.ModelName, "state", status.State)
	return status, nil
}

func (g *gateway) isTritonHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking Triton server health", "server", serverName)

	// Build Triton health URL
	url := fmt.Sprintf("http://%s-endpoint/v2/health/ready", serverName)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create health request: %w", err)
	}

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		logger.Info("Triton health check failed", "server", serverName, "error", err)
		return false, nil // Don't return error, just not healthy
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode == http.StatusOK
	logger.Info("Triton health check completed", "server", serverName, "healthy", healthy)

	return healthy, nil
}
