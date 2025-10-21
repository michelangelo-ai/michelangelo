//go:generate mamockgen AssignmentEngine
package framework

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// AssignmentStrategy decides assignment for a job.
type AssignmentStrategy interface {
	// Select decides an assignment for the given job.
	// Returns (assignment, found, reason, err)
	Select(ctx context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error)
}
