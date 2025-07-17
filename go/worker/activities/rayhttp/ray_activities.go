package rayhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	RayJob ray.RayJob `json:"rayJob"`
}

// CreateRayJobResponse wraps the response from creating a Ray job.
type CreateRayJobResponse struct {
	Object map[string]interface{} `json:"object"`
}

// GetRayJobRequest defines parameters for getting a Ray job.
type GetRayJobRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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
	resp, err := r.httpClient.Post(url, "application/json", bytes.NewReader(rayJobBytes))
	if err != nil {
		logger.Error(err, "activity-error")
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

	response := &CreateRayJobResponse{
		Object: rayJobData,
	}

	return response, nil
}

// GetRayJob retrieves details of a Ray job via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - *GetRayJobResponse: Response containing the job details.
// - error: Error information if the operation fails.
func (r *activities) GetRayJob(ctx context.Context, request GetRayJobRequest) (*GetRayJobResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	if request.Name == "" {
		return nil, errors.New("ray job name is required")
	}

	// Make HTTP GET request to the Ray API using the correct API format
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/rayjobs/%s", r.apiBaseURL, r.workspace, r.environment, request.Name)
	resp, err := r.httpClient.Get(url)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to get ray job", resp.StatusCode)
	}

	var rayJobData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rayJobData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	response := &GetRayJobResponse{
		Object: rayJobData,
	}

	terminalStates := map[string]bool{"SUCCEEDED": true, "FAILED": true, "STOPPED": true}

	object, ok := rayJobData["object"].(map[string]interface{})
	if !ok {
		return nil, workflow.NewCustomError(ctx, "FAILED_PRECONDITION", "no object in response")
	}

	status, ok := object["status"].(map[string]interface{})
	if !ok {
		return nil, workflow.NewCustomError(ctx, "FAILED_PRECONDITION", "no status in object")
	}

	jobStatus, ok := status["state"].(string)
	if !ok {
		return nil, workflow.NewCustomError(ctx, "FAILED_PRECONDITION", status)
	}

	if terminalStates[jobStatus] {
		return nil, workflow.NewCustomError(ctx, "FAILED_PRECONDITION", "job is not in a terminal state")
	}

	return response, nil
}

// ListRayJobs lists all Ray jobs in a namespace via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the namespace.
//
// Returns:
// - *ray.ListRayJobsResponse: Response containing the list of jobs.
// - error: Error information if the operation fails.
func (r *activities) ListRayJobs(ctx context.Context, request ListRayJobsRequest) (*ray.ListRayJobsResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Implement HTTP client call to list Ray jobs
	// This is a placeholder - actual implementation would use the HTTP client to get from the Ray API

	// Simulating a successful response with empty list
	response := &ray.ListRayJobsResponse{
		Items: []unstructured.Unstructured{},
	}

	return response, nil
}

// DeleteRayJob deletes a Ray job via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - bool: True if deletion was successful.
// - error: Error information if the operation fails.
func (r *activities) DeleteRayJob(ctx context.Context, request GetRayJobRequest) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Implement HTTP client call to delete a Ray job
	// This is a placeholder - actual implementation would use the HTTP client to delete from the Ray API

	if request.Name == "" {
		return false, errors.New("ray job name is required")
	}

	// Simulating a successful deletion
	return true, nil
}

// TerminateRayJob terminates a Ray job via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name, namespace, and termination details.
//
// Returns:
// - bool: True if termination was successful.
// - error: Error information if the operation fails.
func (r *activities) TerminateRayJob(ctx context.Context, request struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Implement HTTP client call to terminate a Ray job
	// This is a placeholder - actual implementation would use the HTTP client to post termination to the Ray API

	if request.Name == "" {
		return false, errors.New("ray job name is required")
	}

	// Simulating a successful termination
	return true, nil
}

// CreateRayCluster creates a new Ray cluster via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing the Ray cluster specification.
//
// Returns:
// - *CreateRayClusterResponse: Response containing the created Ray cluster details.
// - error: Error information if the operation fails.
func (r *activities) CreateRayCluster(ctx context.Context, request CreateRayClusterRequest) (*CreateRayClusterResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Extract metadata from the cluster spec
	metadata, ok := request.ClusterSpec["metadata"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid cluster spec: missing metadata")
	}

	// Extract namespace from metadata
	namespace := ""
	if ns, ok := metadata["namespace"].(string); ok {
		namespace = ns
	}
	if namespace == "" {
		return nil, errors.New("namespace is required in cluster spec metadata")
	}

	// Convert cluster spec to JSON for HTTP POST
	clusterBytes, err := json.Marshal(request.ClusterSpec)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Make HTTP POST request to create the Ray cluster using the correct API format
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/rayclusters", r.apiBaseURL, r.workspace, r.environment)
	resp, err := r.httpClient.Post(url, "application/json", bytes.NewReader(clusterBytes))
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to create ray cluster", resp.StatusCode)
	}

	var rayClusterData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rayClusterData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	response := &CreateRayClusterResponse{
		Object: rayClusterData,
	}

	return response, nil
}

// GetRayCluster retrieves details of a Ray cluster via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name and namespace.
//
// Returns:
// - *GetRayClusterResponse: Response containing the cluster details.
// - error: Error information if the operation fails.
func (r *activities) GetRayCluster(ctx context.Context, request GetRayClusterRequest) (*GetRayClusterResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	if request.Name == "" {
		return nil, errors.New("ray cluster name is required")
	}

	// Make HTTP GET request to the Ray API using the correct API format
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/rayclusters/%s", r.apiBaseURL, r.workspace, r.environment, request.Name)
	resp, err := r.httpClient.Get(url)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to get ray cluster", resp.StatusCode)
	}

	var rayClusterData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rayClusterData); err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	response := &GetRayClusterResponse{
		Object: rayClusterData,
	}

	return response, nil
}

// TerminateRayCluster terminates a Ray cluster via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the cluster name, namespace, and termination details.
//
// Returns:
// - bool: True if termination was successful.
// - error: Error information if the operation fails.
func (r *activities) TerminateRayCluster(ctx context.Context, request TerminateRayClusterRequest) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("ray-http-activity-start", zap.Any("request", request))

	// Implement HTTP client call to terminate a Ray cluster
	// This is a placeholder - actual implementation would use the HTTP client to post termination to the Ray API

	if request.Name == "" {
		return false, errors.New("ray cluster name is required")
	}

	// Simulating a successful termination
	return true, nil
}
