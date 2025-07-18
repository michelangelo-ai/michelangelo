package rayhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/worker/ray"
)

var Activities = (*activities)(nil)

// extractJobName extracts the job name from the API response object.
// Returns the name from response["object"]["metadata"]["name"]
func extractJobName(responseObject map[string]interface{}) (string, error) {
	if object, ok := responseObject["object"].(map[string]interface{}); ok {
		if metadata, ok := object["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				return name, nil
			}
		}
	}
	return "", errors.New("could not extract job name from response")
}

// activities struct encapsulates the HTTP client for Ray operations.
type activities struct {
	httpClient  *http.Client
	apiBaseURL  string
	workspace   string
	environment string
}

// CreateRayJobRequest wraps the RayJob for creating a new Ray job.
type CreateRayJobRequest struct {
	RayJob    ray.RayJob `json:"rayJob"`
	UserToken string     `json:"userToken"`
}

// GetRayJobRequest defines parameters for getting a Ray job.
type GetRayJobRequest struct {
	Name      string `json:"name"`
	UserToken string `json:"userToken"`
}

// GetRayJobResponse wraps the response from getting a Ray job.
type GetRayJobResponse struct {
	Object map[string]interface{} `json:"object"`
}

// ListRayJobsRequest defines parameters for listing Ray jobs.
type ListRayJobsRequest struct {
	Namespace string `json:"namespace"`
}

// CreateRayClusterRequest wraps the RayCluster for creating a new Ray cluster.
type CreateRayClusterRequest struct {
	ClusterSpec map[string]interface{} `json:"clusterSpec"`
}

// CreateRayClusterResponse wraps the response from creating a Ray cluster.
type CreateRayClusterResponse struct {
	Object map[string]interface{} `json:"object"`
}

// GetRayClusterRequest defines parameters for getting a Ray cluster.
type GetRayClusterRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// GetRayClusterResponse wraps the response from getting a Ray cluster.
type GetRayClusterResponse struct {
	Object map[string]interface{} `json:"object"`
}

// TerminateRayClusterRequest defines parameters for terminating a Ray cluster.
type TerminateRayClusterRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
}

// CreateRayJob creates a new Ray job using the provided request parameters via HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the Ray job to create.
//
// Returns:
// - *ray.CreateRayJobResponse: Response containing the created Ray job details.
// - error: Error information if the operation fails.
func (r *activities) CreateRayJob(ctx context.Context, request CreateRayJobRequest) (*ray.CreateRayJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Convert RayJob to JSON for HTTP POST
	rayJobBytes, err := json.Marshal(request.RayJob)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Make HTTP POST request to create the Ray job using the correct API format
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/rayjobs", r.apiBaseURL, r.workspace, r.environment)
	req, err := http.NewRequest("POST", url, bytes.NewReader(rayJobBytes))
	if err != nil {
		logger.Error(err, "activity-error: failed to create request")
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", request.UserToken))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "activity-error: failed to execute request")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to create ray job", resp.StatusCode)
	}

	// Read response body as string first
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("ray-http-create-response", zap.String("response", responseBodyStr))

	// Decode string to map first to extract the "object"
	var httpResponse map[string]interface{}
	if unmarshalErr := json.Unmarshal(bodyBytes, &httpResponse); unmarshalErr != nil {
		logger.Error(unmarshalErr, "activity-error decoding response", zap.String("response", responseBodyStr))
		return nil, unmarshalErr
	}

	// Extract the "object" from the HTTP response and return it directly
	// This aligns with the real ray.CreateRayJobResponse structure
	objectData, ok := httpResponse["object"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing object in HTTP response")
	}

	response := &ray.CreateRayJobResponse{
		Object: objectData,
	}

	return response, nil
}

// SensorRayJob retrieves details of a Ray job via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - *GetRayJobResponse: Response containing the job details.
// - error: Error information if the operation fails.
func (r *activities) SensorRayJob(ctx context.Context, request GetRayJobRequest) (*GetRayJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	if request.Name == "" {
		return nil, errors.New("ray job name is required")
	}

	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/rayjobs/%s", r.apiBaseURL, r.workspace, r.environment, request.Name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error(err, "activity-error: failed to get request")
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", request.UserToken))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "activity-error: failed to execute request")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to get ray job", resp.StatusCode)
	}

	// Read response body as string first for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("ray-http-sensor-response", zap.String("response", responseBodyStr))

	// Decode the full HTTP response which includes the "object" wrapper
	var httpResponse map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &httpResponse); err != nil {
		logger.Error(err, "activity-error decoding response", zap.String("response", responseBodyStr))
		return nil, err
	}

	// Extract the "object" from the HTTP response (same as CreateRayJob does)
	objectData, ok := httpResponse["object"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing object in HTTP response")
	}

	// Check if the job has reached a terminal state
	if status, ok := objectData["status"].(map[string]interface{}); ok {
		if jobStatus, ok := status["jobStatus"].(string); ok {
			if jobStatus == "SUCCEEDED" || jobStatus == "FAILED" {
				return &GetRayJobResponse{
					Object: objectData,
				}, nil
			}
		}
	}

	// If we can't determine status, assume it's not ready yet
	logger.Info("ray-job-status-unknown", zap.String("jobName", request.Name))
	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), "unknown status")
}
