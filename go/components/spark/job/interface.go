package job

import (
	"context"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Client is an interface for managing Spark jobs. It provides methods to create jobs
// and retrieve their statuses. This interface abstracts the interaction with Spark,
// allowing for easier testing and integration.
type Client interface {

	// CreateJob creates a new Spark job.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - log: A logger instance for logging information.
	// - job: A pointer to a SparkJob object containing job details.
	//
	// Returns:
	// - An error if the job creation fails.
	CreateJob(ctx context.Context, log logr.Logger, job *v2pb.SparkJob) error

	// GetJobStatus retrieves the status of a Spark job.
	//
	// Parameters:
	// - ctx: The context for managing request deadlines and cancellations.
	// - logger: A logger instance for logging information.
	// - job: A pointer to a SparkJob object for which the status is being retrieved.
	//
	// Returns:
	// - A pointer to a string representing the job status (state).
	// - A string containing the job URL.
	// - A string containing the error message (if the job failed).
	// - An error if the status retrieval fails.
	GetJobStatus(ctx context.Context, logger logr.Logger, job *v2pb.SparkJob) (*string, string, string, error)
}
