package serving

import (
	"context"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Provider is an interface for managing InferenceServer resources. It provides methods to create,
// update, monitor, and retire InferenceServer instances across different inference serving platforms.
// This interface abstracts the interaction with various serving platforms (Triton, custom inference servers, etc.),
// allowing for easier testing, integration, and multi-platform support.
type Provider interface {

	// CreateInferenceServer creates a new InferenceServer resource on the target platform.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - name: Name of the inference server.
	// - namespace: Namespace for the inference server.
	//
	// Returns:
	// - An error if the InferenceServer creation fails.
	CreateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error

	// UpdateInferenceServer handles InferenceServer updates including resource changes,
	// serving configuration updates, and other specification modifications.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - name: Name of the inference server.
	// - namespace: Namespace for the inference server.
	//
	// Returns:
	// - An error if the update operation fails.
	UpdateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error

	// GetStatus retrieves the current status of an InferenceServer from the serving platform
	// and directly updates the status field in the InferenceServer proto object.
	// This method queries the platform-specific APIs to determine InferenceServer health,
	// readiness, and other status information.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - logger: A logger instance for logging information.
	// - inferenceServer: The InferenceServer proto object to update with status
	//
	// Returns:
	// - error: An error if the status retrieval fails.
	GetStatus(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error

	// DeleteInferenceServer cleanly shuts down and removes an InferenceServer from the serving platform.
	// This includes cleaning up associated resources like services, deployments,
	// and platform-specific configurations.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - name: Name of the inference server.
	// - namespace: Namespace for the inference server.
	//
	// Returns:
	// - An error if the deletion operation fails.
	DeleteInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error
}
