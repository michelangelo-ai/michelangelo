package provider

import (
	"context"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Client is an interface for managing Spark jobs. It provides methods to create jobs
// and retrieve their statuses. This interface abstracts the interaction with Spark,
// allowing for easier testing and integration.
type Provider interface {

	// CreateJob creates a new Spark job.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - job: A pointer to a SparkJob object containing job details.
	//
	// Returns:
	// - An error if the job creation fails.
	CreateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error

	Rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error

	// GetJobStatus retrieves the status of a Spark job.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - logger: A logger instance for logging information.
	// - job: A pointer to a SparkJob object for which the status is being retrieved.
	//
	// Returns:
	// - A pointer to a string representing the job status.
	// - A string containing additional status information.
	// - An error if the status retrieval fails.
	GetStatus(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error

	Retire(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error
}
