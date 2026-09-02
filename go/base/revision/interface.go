package revision

import (
	"context"
)

// Manager handles the Revision CR lifecycle with immutability semantics.
// Controllers that produce revisions depend on this interface rather than
// calling the API handler directly.
type Manager interface {
	// UpsertRevision creates or updates a Revision built from the given input.
	// The Manager marshals the BaseCR, derives BaseType and BaseResource, and
	// orchestrates the state machine (get existing, create if absent, check
	// immutability, update if mutable).
	// Returns (true, nil) on create or update, (false, nil) on dedup (an
	// existing immutable Revision with the same name already exists).
	UpsertRevision(ctx context.Context, input UpsertParams, opts UpsertOpts) (bool, error)
}

// UpsertOpts carries state-machine knobs for UpsertRevision.
type UpsertOpts struct {
	Immutable bool
}
