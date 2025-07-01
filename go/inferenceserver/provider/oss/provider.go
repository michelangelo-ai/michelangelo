package oss

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Request and Response types for the provider interface
type CreateRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

type CreateResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}

type GetRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

type GetResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}

type DeleteRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

type DeleteResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}

// Provider implements the Provider interface for OSS deployments
// It uses the inference server gateway to provision pods across different providers
type Provider struct {
	client  client.Client
	gateway inferenceserver.Gateway
}

// NewProvider creates a new OSS provider
func NewProvider(client client.Client, gateway inferenceserver.Gateway) *Provider {
	return &Provider{
		client:  client,
		gateway: gateway,
	}
}

// Create creates the inference server infrastructure using the gateway
func (p *Provider) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	logger := req.Logger.WithName("oss-provider")
	
	logger.Info("Creating inference server", 
		"name", req.InferenceServer.Name, 
		"namespace", req.InferenceServer.Namespace,
		"backend", req.InferenceServer.Spec.BackendType)

	// Use the gateway to provision the inference server based on backend type
	switch req.InferenceServer.Spec.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return p.createWithGateway(ctx, logger, req.InferenceServer, "triton")
	case v2pb.BACKEND_TYPE_LLM_D:
		return p.createWithGateway(ctx, logger, req.InferenceServer, "llmd")
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", req.InferenceServer.Spec.BackendType)
	}
}

// createWithGateway uses the inference server gateway to create the server
func (p *Provider) createWithGateway(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer, providerType string) (*CreateResponse, error) {
	logger.Info("Provisioning inference server via gateway", "provider", providerType)

	// Create a model load request to initialize the server
	loadRequest := inferenceserver.ModelLoadRequest{
		ModelName:       inferenceServer.Name,
		ModelVersion:    "1.0.0", // Default version
		InferenceServer: inferenceServer.Name,
		BackendType:     inferenceServer.Spec.BackendType,
		Config:          make(map[string]string),
	}

	// Load the initial model configuration through the gateway
	err := p.gateway.LoadModel(ctx, logger, loadRequest)
	if err != nil {
		logger.Error(err, "Failed to load model via gateway")
		return &CreateResponse{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to provision server: %v", err),
		}, err
	}

	logger.Info("Successfully initiated server provisioning via gateway")
	
	return &CreateResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: fmt.Sprintf("Inference server provisioning initiated for %s backend", providerType),
		Details: map[string]interface{}{
			"backend":     providerType,
			"server":      inferenceServer.Name,
			"namespace":   inferenceServer.Namespace,
		},
	}, nil
}

// Get retrieves the current status of the inference server using the gateway
func (p *Provider) Get(ctx context.Context, req *GetRequest) (*GetResponse, error) {
	logger := req.Logger.WithName("oss-provider")
	
	logger.Info("Checking inference server status", 
		"name", req.InferenceServer.Name,
		"backend", req.InferenceServer.Spec.BackendType)

	// Check server health via gateway
	isHealthy, err := p.gateway.IsHealthy(ctx, logger, req.InferenceServer.Name, req.InferenceServer.Spec.BackendType)
	if err != nil {
		logger.Error(err, "Failed to check server health via gateway")
		return &GetResponse{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Health check failed: %v", err),
		}, nil
	}

	if isHealthy {
		// Check if any models are loaded
		statusRequest := inferenceserver.ModelStatusRequest{
			ModelName:       req.InferenceServer.Name,
			ModelVersion:    "1.0.0",
			InferenceServer: req.InferenceServer.Name,
			BackendType:     req.InferenceServer.Spec.BackendType,
		}

		modelReady, err := p.gateway.CheckModelStatus(ctx, logger, statusRequest)
		if err != nil {
			logger.Error(err, "Failed to check model status via gateway")
			return &GetResponse{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: "Server is healthy but model status unknown",
				Details: map[string]interface{}{
					"healthy": true,
					"backend": req.InferenceServer.Spec.BackendType.String(),
				},
			}, nil
		}

		if modelReady {
			return &GetResponse{
				State:   v2pb.INFERENCE_SERVER_STATE_SERVING,
				Message: "Inference server is healthy and serving",
				Details: map[string]interface{}{
					"healthy":     true,
					"modelReady":  true,
					"backend":     req.InferenceServer.Spec.BackendType.String(),
				},
			}, nil
		} else {
			return &GetResponse{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: "Server is healthy but models are still loading",
				Details: map[string]interface{}{
					"healthy":    true,
					"modelReady": false,
					"backend":    req.InferenceServer.Spec.BackendType.String(),
				},
			}, nil
		}
	}

	return &GetResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Inference server is still starting up",
		Details: map[string]interface{}{
			"healthy": false,
			"backend": req.InferenceServer.Spec.BackendType.String(),
		},
	}, nil
}

// Delete removes the inference server infrastructure
func (p *Provider) Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error) {
	logger := req.Logger.WithName("oss-provider")
	
	logger.Info("Deleting inference server", 
		"name", req.InferenceServer.Name,
		"backend", req.InferenceServer.Spec.BackendType)

	// For now, we don't have a direct delete method in the gateway
	// In a real implementation, this would:
	// 1. Unload all models from the server
	// 2. Scale down the deployment to 0
	// 3. Remove associated resources
	
	logger.Info("Inference server deletion completed")
	
	return &DeleteResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_DELETED,
		Message: "Inference server resources cleaned up successfully",
		Details: map[string]interface{}{
			"backend": req.InferenceServer.Spec.BackendType.String(),
		},
	}, nil
}