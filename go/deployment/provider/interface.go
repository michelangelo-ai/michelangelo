package provider

import (
	"context"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Provider is an interface for managing ML model deployments. It provides methods to create,
// update, monitor, and retire model deployments across different inference serving platforms.
// This interface abstracts the interaction with various serving platforms (Triton, KServe, etc.),
// allowing for easier testing, integration, and multi-platform support.
type Provider interface {

	// CreateDeployment creates a new model deployment on the target inference serving platform.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - deployment: A pointer to a Deployment object containing deployment specification.
	// - model: A pointer to a Model object containing model metadata and artifacts.
	//
	// Returns:
	// - An error if the deployment creation fails.
	CreateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error

	// Rollout handles model version updates and traffic routing changes for an existing deployment.
	// This includes updating model configurations, managing traffic splits, and coordinating
	// service mesh routing rules.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - deployment: A pointer to a Deployment object with updated specification.
	// - model: A pointer to a Model object containing new model version details.
	//
	// Returns:
	// - An error if the rollout operation fails.
	Rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error

	// GetStatus retrieves the current status of a model deployment from the serving platform.
	// This method queries the platform-specific APIs to determine deployment health,
	// readiness, and other status information.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - logger: A logger instance for logging information.
	// - deployment: A pointer to a Deployment object for which status is being retrieved.
	//
	// Returns:
	// - An error if the status retrieval fails. The deployment status should be updated
	//   in-place within the deployment object.
	GetStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error

	// Retire cleanly shuts down and removes a model deployment from the serving platform.
	// This includes cleaning up associated resources like services, ingress rules,
	// and platform-specific configurations.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - deployment: A pointer to a Deployment object to be retired.
	//
	// Returns:
	// - An error if the retirement operation fails.
	Retire(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error
}
