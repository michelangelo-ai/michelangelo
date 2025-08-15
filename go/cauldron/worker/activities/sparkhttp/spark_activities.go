package sparkhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/compute/spark"
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

// buildRequirementFilePath constructs the S3 path for the requirement file
func buildRequirementFilePath(workspace, username, pipeline, name string) string {
	// Replace hyphens with underscores in the name
	sanitizedName := strings.ReplaceAll(name, "-", "_")

	return fmt.Sprintf("s3://chimera-mlpipeline/artifact/%s/%s/pipelines/%s/uniflow/%s/requirements-compiled.txt",
		workspace, username, pipeline, sanitizedName)
}

// activities struct encapsulates the HTTP client for Spark operations.
type activities struct {
	httpClient   *http.Client
	apiBaseURL   string
	workspace    string
	environment  string
	sparkDepsURL string
}

// CreateSparkOneRequest wraps the SparkOne for creating a new SparkOne job.
type CreateSparkOneRequest struct {
	SparkOne  spark.SparkOne `json:"sparkOne"`
	UserToken string         `json:"userToken"`
}

// GetSparkOneRequest defines parameters for getting a SparkOne.
type GetSparkOneRequest struct {
	Name      string `json:"name"`
	UserToken string `json:"userToken"`
}

// GetSparkOneResponse wraps the response from getting a SparkOne.
type GetSparkOneResponse struct {
	Object map[string]interface{} `json:"object"`
}

// ListSparkOnesRequest defines parameters for listing SparkOnes.
type ListSparkOnesRequest struct {
	Namespace string `json:"namespace"`
}

// CreateSparkOneDepsRequest defines parameters for creating SparkOne dependencies.
type CreateSparkOneDepsRequest struct {
	Username string `json:"username"`
	Pipeline string `json:"pipeline"`
	JobName  string `json:"jobName"`
}

// CreateSparkOneDepsResponse wraps the response from creating SparkOne dependencies.
type CreateSparkOneDepsResponse struct {
	S3Path  string `json:"s3path"`
	PollURL string `json:"pollUrl"`
	Msg     string `json:"msg"`
}

// SensorSparkOneDepsRequest defines parameters for checking SparkOne dependencies status.
type SensorSparkOneDepsRequest struct {
	PollURL string `json:"pollUrl"`
}

// SensorSparkOneDepsResponse wraps the response from checking SparkOne dependencies status.
type SensorSparkOneDepsResponse struct {
	S3Path string `json:"s3path"`
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

// CreateSparkOne creates a new SparkOne using the provided request parameters via HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing details of the SparkOne to create.
//
// Returns:
// - *spark.CreateSparkOneResponse: Response containing the created SparkOne details.
// - error: Error information if the operation fails.
func (r *activities) CreateSparkOne(ctx context.Context, request CreateSparkOneRequest) (*spark.CreateSparkOneResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("spark-http-activity-start", zap.Any("request", request))

	// Convert SparkOne to JSON for HTTP POST
	sparkOneBytes, err := json.Marshal(request.SparkOne)
	if err != nil {
		logger.Error(err, "activity-error")
		return nil, err
	}

	// Make HTTP POST request to create the SparkOne using the correct API format
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/sparkones", r.apiBaseURL, r.workspace, r.environment)
	req, err := http.NewRequest("POST", url, bytes.NewReader(sparkOneBytes))
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
		return nil, fmt.Errorf("HTTP %d: failed to create sparkone", resp.StatusCode)
	}

	// Read response body as string first
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("spark-http-create-response", zap.String("response", responseBodyStr))

	// Decode string to map first to extract the "object"
	var httpResponse map[string]interface{}
	if unmarshalErr := json.Unmarshal(bodyBytes, &httpResponse); unmarshalErr != nil {
		logger.Error(unmarshalErr, "activity-error decoding response", zap.String("response", responseBodyStr))
		return nil, unmarshalErr
	}

	// Extract the "object" from the HTTP response and return it directly
	// This aligns with the real spark.CreateSparkOneResponse structure
	objectData, ok := httpResponse["object"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing object in HTTP response")
	}

	response := &spark.CreateSparkOneResponse{
		Object: objectData,
	}

	return response, nil
}

// SensorSparkOne retrieves details of a SparkOne via HTTP API.
//
// Params:
// - ctx: The context for the operation.
// - request: Request containing the job name and namespace.
//
// Returns:
// - *GetSparkOneResponse: Response containing the job details.
// - error: Error information if the operation fails.
func (r *activities) SensorSparkOne(ctx context.Context, request GetSparkOneRequest) (*GetSparkOneResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("spark-http-activity-start", zap.Any("request", request))

	if request.Name == "" {
		return nil, errors.New("spark job name is required")
	}

	url := fmt.Sprintf("%s/api/v1/workspaces/%s/env/%s/sparkones/%s", r.apiBaseURL, r.workspace, r.environment, request.Name)
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
		return nil, fmt.Errorf("HTTP %d: failed to get sparkone", resp.StatusCode)
	}

	// Read response body as string first for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("spark-http-sensor-response", zap.String("response", responseBodyStr))

	// Decode the full HTTP response which includes the "object" wrapper
	var httpResponse map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &httpResponse); err != nil {
		logger.Error(err, "activity-error decoding response", zap.String("response", responseBodyStr))
		return nil, err
	}

	// Extract the "object" from the HTTP response (same as CreateSparkOne does)
	objectData, ok := httpResponse["object"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing object in HTTP response")
	}

	// Check if the job has reached a terminal state
	if status, ok := objectData["status"].(map[string]interface{}); ok {
		if jobStatus, ok := status["status"].(string); ok {
			if jobStatus == "SUCCEEDED" || jobStatus == "FAILED" {
				return &GetSparkOneResponse{
					Object: objectData,
				}, nil
			}
		}
	}

	// If we can't determine status, assume it's not ready yet
	logger.Info("spark-job-status-unknown", zap.String("jobName", request.Name))
	return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), "unknown status")
}

// CreateSparkOneDeps creates SparkOne dependencies via the Mjolnir HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing the requirement file path.
//
// Returns:
// - *CreateSparkOneDepsResponse: Response containing the pollUrl and status.
// - error: Error information if the operation fails.
func (r *activities) CreateSparkOneDeps(ctx context.Context, request CreateSparkOneDepsRequest) (*CreateSparkOneDepsResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("spark-http-deps-activity-start", zap.Any("request", request))

	if request.Username == "" || request.Pipeline == "" || request.JobName == "" {
		return nil, errors.New("username, pipeline, and jobName are required")
	}

	// Build requirement file path using workspace from activities configuration
	requirementFile := buildRequirementFilePath(r.workspace, request.Username, request.Pipeline, request.JobName)
	logger.Info("constructed-requirement-file-path", zap.String("path", requirementFile))

	// Create the request body for Mjolnir API
	requestBody := map[string]interface{}{
		"requirement_file": requirementFile,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error(err, "activity-error: failed to marshal request body")
		return nil, err
	}

	// Make HTTP POST request to Mjolnir API
	req, err := http.NewRequest("POST", r.sparkDepsURL+"/v1/environment", bytes.NewReader(jsonBody))
	if err != nil {
		logger.Error(err, "activity-error: failed to create deps request")
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "activity-error: failed to execute deps request")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("HTTP %d: failed to create sparkone deps", resp.StatusCode)
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading deps response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("spark-http-deps-create-response", zap.String("response", responseBodyStr))

	// Decode the response
	var response CreateSparkOneDepsResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		logger.Error(err, "activity-error decoding deps response", zap.String("response", responseBodyStr))
		return nil, err
	}

	return &response, nil
}

// SensorSparkOneDeps checks the status of SparkOne dependencies via the Mjolnir HTTP API.
//
// This method is executed as part of a Starlark worker activity.
//
// Params:
// - ctx: The context for the operation.
// - request: The request containing the pollUrl from CreateSparkOneDeps.
//
// Returns:
// - *SensorSparkOneDepsResponse: Response containing the status and S3 path.
// - error: Error information if the operation fails.
func (r *activities) SensorSparkOneDeps(ctx context.Context, request SensorSparkOneDepsRequest) (*SensorSparkOneDepsResponse, error) {
	logger := log.FromContext(ctx)
	logger.Info("spark-http-deps-sensor-activity-start", zap.Any("request", request))

	if request.PollURL == "" {
		return nil, errors.New("poll URL is required")
	}

	// Make HTTP GET request to check status
	req, err := http.NewRequest("GET", request.PollURL, nil)
	if err != nil {
		logger.Error(err, "activity-error: failed to create deps sensor request")
		return nil, err
	}

	// Set headers
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "activity-error: failed to execute deps sensor request")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: failed to check sparkone deps status", resp.StatusCode)
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "activity-error reading deps sensor response body")
		return nil, err
	}

	responseBodyStr := string(bodyBytes)
	logger.Info("spark-http-deps-sensor-response", zap.String("response", responseBodyStr))

	// Decode the response
	var response SensorSparkOneDepsResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		logger.Error(err, "activity-error decoding deps sensor response", zap.String("response", responseBodyStr))
		return nil, err
	}

	// Check if dependencies are ready
	if response.Status == "success" {
		return &response, nil
	} else if response.Status == "running" || response.Status == "pending" {
		// Return a retriable error to continue polling
		logger.Info("deps-still-building", zap.String("status", response.Status))
		return nil, workflow.NewCustomError(ctx, yarpcerrors.CodeFailedPrecondition.String(), "dependencies still building")
	} else {
		// Failed status
		return nil, fmt.Errorf("dependency build failed with status: %s, msg: %s", response.Status, response.Msg)
	}
}
