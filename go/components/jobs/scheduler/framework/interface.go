//go:generate mamockgen AssignmentEngine
package framework

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// AssignmentStrategy decides assignment for a job.
type AssignmentStrategy interface {
	// Select decides an assignment for the given job.
	// Returns (assignment, found, reason, err)
	Select(ctx context.Context, job BatchJob) (*v2pb.AssignmentInfo, bool, string, error)
}

// ExcludingAssignmentStrategy is an AssignmentStrategy that can also decide
// with clusters excluded, as a job re-placed off earlier targets needs
// (RFC 0f). It is optional: callers check for it, and decide with Select
// otherwise.
type ExcludingAssignmentStrategy interface {
	AssignmentStrategy
	// SelectExcluding decides as Select does, except that the excluded
	// clusters are not candidates; a pin is kept even when excluded. It
	// returns found=false when the exclusion leaves no candidate, and the
	// caller then decides again with Select.
	SelectExcluding(ctx context.Context, job BatchJob, excluded []string) (*v2pb.AssignmentInfo, bool, string, error)
}
