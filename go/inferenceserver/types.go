package inferenceserver

import (
	"context"
	
	"github.com/go-logr/logr"
	
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Provider interface for infrastructure-specific implementations
type Provider interface {
	// Create creates the inference server infrastructure
	Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error)
	
	// Get retrieves the current status of the inference server
	Get(ctx context.Context, req *GetRequest) (*GetResponse, error)
	
	// Delete removes the inference server infrastructure
	Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error)
}

// CreateRequest contains the parameters for creating an inference server
type CreateRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

// CreateResponse contains the result of creating an inference server
type CreateResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}

// GetRequest contains the parameters for getting inference server status
type GetRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

// GetResponse contains the current status of an inference server
type GetResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}

// DeleteRequest contains the parameters for deleting an inference server
type DeleteRequest struct {
	InferenceServer *v2pb.InferenceServer
	Logger          logr.Logger
}

// DeleteResponse contains the result of deleting an inference server
type DeleteResponse struct {
	State   v2pb.InferenceServerState
	Message string
	Details map[string]interface{}
}