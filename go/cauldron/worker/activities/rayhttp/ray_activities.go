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

	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/compute/ray"
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

// CreateRayJobResponse wraps the response from creating a Ray job.
type CreateRayJobResponse struct {
	Object map[string]interface{} `json:"object"`
}

// BuildRayJobImageRequest defines parameters for building a Ray job image.
type BuildRayJobImageRequest struct {
	Object           map[string]interface{} `json:"Object"`
	UsePipelineImage bool                   `json:"UsePipelineImage"`
	CommitHash       string                 `json:"CommitHash"`
	UserToken        string                 `json:"userToken"`
}

// BuildRayJobImageResponse wraps the response from building a Ray job image.
type BuildRayJobImageResponse struct {
	JobName          string   `json:"jobName"`
	Status           string   `json:"status"`
	ImageRegistry    string   `json:"imageRegistry"`
	ImageTag         []string `json:"imageTag"`
	PipelineS3Prefix string   `json:"pipelineS3Prefix"`
}

// SensorRayJobImageRequest defines parameters for sensing Ray job image build status.
type SensorRayJobImageRequest struct {
	JobName   string `json:"jobName"`
	UserToken string `json:"userToken"`
}

// SensorRayJobImageResponse wraps the response from sensing Ray job image build status.
type SensorRayJobImageResponse struct {
	Status map[string]interface{} `json:"status"`
}

// GetRayJobRequest defines parameters for getting a Ray job.
type GetRayJobRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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

// BuildRayJobImage builds a Ray job image using the provided request parameters via HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the Ray job image to build.
//
// Returns:
// - *BuildRayJobImageResponse: Response containing the build job details.
// - error: Error information if the operation fails.
func (r *activities) BuildRayJobImage(ctx context.Context, request BuildRayJobImageRequest) (*BuildRayJobImageResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-build-image-activity-start", zap.Any("request", request))

	// Convert request to JSON for HTTP POST
	requestBody := map[string]interface{}{
		"Object":           request.Object,
		"UsePipelineImage": request.UsePipelineImage,
		"CommitHash":       request.CommitHash,
	}

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Make HTTP POST request to build the Ray job image
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/cimages/uniflowtask", r.apiBaseURL, r.workspace, r.environment)
	req, err := http.NewRequest("POST", url, bytes.NewReader(requestBytes))
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
		return nil, fmt.Errorf("HTTP %d: failed to build ray job image", resp.StatusCode)
	}

	var responseData BuildRayJobImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Check if the build was initiated successfully
	if responseData.Status != "succeed" {
		return nil, fmt.Errorf("ray job image build failed with status: %s", responseData.Status)
	}

	return &responseData, nil
}

// SensorRayJobImage senses the status of a Ray job image build via HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing the job name to sense.
//
// Returns:
// - *SensorRayJobImageResponse: Response containing the build status.
// - error: Error information if the operation fails.
func (r *activities) SensorRayJobImage(ctx context.Context, request SensorRayJobImageRequest) (*SensorRayJobImageResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-sensor-image-activity-start", zap.Any("request", request))

	if request.JobName == "" {
		return nil, errors.New("job name is required")
	}

	// Make HTTP GET request to sense the Ray job image build status
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/cimages/status/%s", r.apiBaseURL, r.workspace, r.environment, request.JobName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error(err, "activity-error: failed to create request")
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", request.UserToken))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "activity-error: failed to execute request")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to sense ray job image build status", resp.StatusCode)
	}

	var responseData SensorRayJobImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Check if the build is complete
	if status, ok := responseData.Status["succeeded"].(float64); ok && status == 1 {
		return &responseData, nil
	}

	// If build is not complete, return a retryable error
	return nil, workflow.NewCustomError(ctx, "FAILED_PRECONDITION", "image build not complete")
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
// - *CreateRayJobResponse: Response containing the created Ray job details.
// - error: Error information if the operation fails.
func (r *activities) CreateRayJob(ctx context.Context, request CreateRayJobRequest) (*CreateRayJobResponse, error) {
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

	var rayJobData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rayJobData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}
	objectData, ok := rayJobData["object"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing object in HTTP response")
	}

	response := &CreateRayJobResponse{
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
