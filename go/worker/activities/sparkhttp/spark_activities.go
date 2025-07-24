package sparkhttp

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

	"github.com/michelangelo-ai/michelangelo/go/worker/spark"
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

// activities struct encapsulates the HTTP client for Spark operations.
type activities struct {
	httpClient  *http.Client
	apiBaseURL  string
	workspace   string
	environment string
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
